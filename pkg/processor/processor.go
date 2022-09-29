package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	dict "github.com/christiancadieux/kubequery-postgres/pkg/dictionary"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Processor struct {
	db          *sql.DB
	dictio      *dict.Dictionary
	root        string
	largeSize   int
	concurrency int
}

func NewProcessor(db *sql.DB, dictio *dict.Dictionary, root string, concurrency int) (*Processor, error) {
	return &Processor{
		db:          db,
		dictio:      dictio,
		root:        root,
		largeSize:   30000,
		concurrency: concurrency,
	}, nil
}

func (proc *Processor) Run() error {

	workers := make(chan struct{}, proc.concurrency)

	for ix, cluster := range proc.dictio.Clusters {

		workers <- struct{}{}
		go func(cluster *dict.Cluster, items chan struct{}, ix int) {
			defer func() {
				<-items
			}()
			fmt.Printf("%d Cluster START %s - %s \n", ix, cluster.FacName, cluster.Address)

			err, out1 := proc.ProcessCluster(cluster, proc.dictio.Tables, proc.dictio.TableFields, proc.dictio.FieldTypes, ix)

			fmt.Println("------------------------------------------------------------------------------")
			if err != nil {
				fmt.Printf("%d Cluster ERROR %s - %s - %v \n", ix, cluster.FacName, cluster.Address, err)
			} else {
				fmt.Printf("%d Cluster REPORT %s - %s \n", ix, cluster.FacName, cluster.Address)
			}
			fmt.Println(out1)

			if err != nil {
				fmt.Println("Error processing", err)
			}
		}(cluster, workers, ix)
	}

	for i := 0; i < cap(workers); i++ {
		workers <- struct{}{}
	}
	return nil
}

func (proc *Processor) ProcessCluster(cluster *dict.Cluster, tables []*dict.Table, tableFields map[string]string, fieldTypes map[string]string, ix int) (error, string) {

	out := ""
	for _, table := range tables {
		start := time.Now()
		out += fmt.Sprintln(" ", cluster.FacName, table.Name, "TABLE")

		err := proc.ExtractData(cluster, table.Name, ix)
		if err != nil {
			out += fmt.Sprintf("%s %s ExtractData Error - %v", cluster.FacName, table.Name, err)
			continue
		}

		rows, err, out1 := proc.processData(cluster, tableFields, fieldTypes, table.Name, ix)
		out += out1
		if err != nil {
			out += fmt.Sprintln("Error=", err)
		}
		out += fmt.Sprintf("  %s %s Rows=%d, Duration=%v\n", cluster.FacName, table.Name, rows, time.Since(start))
	}

	return nil, out
}

func (proc *Processor) ExtractData(cluster *dict.Cluster, table string, ix int) error {

	cmd := exec.Command(proc.root+"runquery", table,
		cluster.Token,
		cluster.Address,
		cluster.FacName,
		cluster.FacilityID,
		fmt.Sprintf("%d", ix))

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil

}

func (proc *Processor) processData(cluster *dict.Cluster, tableFields map[string]string, fieldTypes map[string]string, table string, ix int) (int, error, string) {

	out := ""
	fieldlist := tableFields[table]
	fields := strings.Split(fieldlist, ",")
	holders := ""
	newfieldlist := ""
	rowId := 1
	for _, f := range fields {
		if holders != "" {
			holders += ","
		}
		holders += "$" + fmt.Sprintf("%d", rowId)
		rowId++
		if newfieldlist != "" {
			newfieldlist += ","
		}
		newfieldlist += f
	}
	insert := fmt.Sprintf("insert into %s (%s) values (%s)", table, newfieldlist, holders)

	data := []map[string]interface{}{}

	fname := fmt.Sprintf("/tmp/%s-%s.json", cluster.FacName, table)
	b, err := os.ReadFile(fname)
	if err != nil {
		return 0, fmt.Errorf("Cannot read file %s - %v", table, err), ""
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return 0, fmt.Errorf("Cannot unmarshal file %s - %v", table, err), ""
	}
	if len(data) == 0 {
		// if no data found, don't delete the current data, just skip.
		out += fmt.Sprintln("", cluster.FacName, table, "NO Data")
		return 0, nil, out
	}

	proc.db.Exec("set autocommit off;")

	ctx := context.Background()
	options := sql.TxOptions{Isolation: sql.LevelDefault}
	tx, err := proc.db.BeginTx(ctx, &options)

	defer tx.Commit()
	delet := fmt.Sprintf("delete from %s where cluster_name = '%s';", table, cluster.FacName)
	proc.db.Exec(delet)

	printcnt := 0
	stmt, err := proc.db.Prepare(insert)
	if err != nil {
		return 0, fmt.Errorf("Cannot prepare %s - %v", insert, err), ""
	}

	for cnt, row := range data {
		// fmt.Printf("row= %+v \n", row)
		args := []interface{}{}

		for _, field := range fields {
			// fmt.Printf("Field %s  = %+v \n", field, row[field])
			ft := fieldTypes[table+":"+field]
			switch row[field].(type) {
			case int:
				if ft == "INTEGER" || ft == "BIGINT" {
					args = append(args, row[field].(int))
				} else {
					v := strconv.Itoa(row[field].(int))
					args = append(args, v)
				}
			case string:
				if ft == "INTEGER" || ft == "BIGINT" {
					if row[field].(string) == "" {
						args = append(args, 0)
					} else {
						v, err := strconv.Atoi(row[field].(string))
						if err != nil {
							out += fmt.Sprintln("  convert error string => int", row[field])
						} else {
							args = append(args, v)
						}
					}
					// string on jsonb (labels)
				} else {
					if len(row[field].(string)) > proc.largeSize {
						out += fmt.Sprintln("  LARGE STRING", field, len(row[field].(string)))
					}
					if field == "labels" {
						if row[field].(string) == "" {
							args = append(args, "{}")
						} else {
							args = append(args, row[field].(string))
						}
					} else {
						args = append(args, row[field].(string))
					}
				}
			}
		}

		_, err = stmt.Exec(args...)
		if err != nil {
			return 0, fmt.Errorf("Exec %v", err), ""
		}

		printcnt++
		if printcnt > 1000 {
			out += fmt.Sprintln(" ", cluster.FacName, table, cnt, "args=", len(args))
			printcnt = 0
		}
	}

	return len(data), nil, out
}
