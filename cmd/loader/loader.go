package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	CONCURRENCY = 1
)


type Cluster struct {
	Token      string `json:"token"`
	Address    string `json:"address"`
	FacName    string `json:"name"`
	FacilityID string `json:"id"`
}

type Table struct {
	Name string `json:"name"`
}

type Dictionary struct {
	Clusters []*Cluster `json:"clusters"`
	Tables   []*Table   `json:"tables"`
}

func getPg() string {
	host := os.Getenv("PG_HOST")
	port, err := strconv.Atoi(os.Getenv("PG_PORT"))
	if err != nil {
		port = 5432
	}
	user := os.Getenv("PG_USER")
	password := os.Getenv("PG_PASS")
	db := os.Getenv("PG_DB")
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, db)
}

func getRoot() string {
	path := os.Getenv("KQ_ROOT")
	if path == "" {
		// running in container
		return "/"
	}
	return path + "/"
}

func loadSchema(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func TrimSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		s = s[:len(s)-len(suffix)]
	}
	return s
}

func parseSchema(tableFields map[string]string, fieldTypes map[string]string, path string) error {
	lines, err := loadSchema(path)
	if err != nil {
		return err
	}
	table := ""
	fieldlist := ""
	for _, line0 := range lines {
		line := strings.TrimSpace(line0)
		words := strings.Fields(line)
		if len(words) < 2 {
			continue
		}
		if words[0] == ");" {
			continue
		}

		if words[0] == "CREATE" {
			if table != "" {
				tableFields[table] = fieldlist
			}
			table = words[2]
			fieldlist = ""
		} else {
			w := strings.ReplaceAll(words[0], "`", "")
			ftype := TrimSuffix(words[1], ",")
			if fieldlist != "" {
				fieldlist += ","
			}
			if w == "group" || w == "default" {
				fieldlist += fmt.Sprintf(`"%s"`, w)
			} else {
				fieldlist += w
			}

			fieldTypes[table+":"+w] = ftype
		}
	}
	return nil

}

func extractData(cluster *Cluster, table string, ix int) {
	cmd := exec.Command(getRoot() + "runquery", table,
		cluster.Token,
		cluster.Address,
		cluster.FacName,
		cluster.FacilityID,
		fmt.Sprintf("%d", ix))

	if err := cmd.Run(); err != nil {
		fmt.Println("Error: ", err)
	}

}

func processData(db *sql.DB, cluster *Cluster, tableFields map[string]string, fieldTypes map[string]string, table string, ix int) (int, error, string) {

	extractData(cluster, table, ix)
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
		/*
			// additional fields
			if f == "node_selector" {
				holders += ",$" + fmt.Sprintf("%d", rowId)
				rowId++
				newfieldlist += ",zone"
			}
		*/
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

	db.Exec("set autocommit off;")

	ctx := context.Background()
	options := sql.TxOptions{Isolation: sql.LevelDefault}
	tx, err := db.BeginTx(ctx, &options)

	defer tx.Commit()
	delet := fmt.Sprintf("delete from %s where cluster_name = '%s';", table, cluster.FacName)
	db.Exec(delet)

	printcnt := 0
	stmt, err := db.Prepare(insert)
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
					if len(row[field].(string)) > 30000 {
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

func getDictionary() (*Dictionary, error) {

	results := Dictionary{}

	b, err := os.ReadFile(getRoot() + "dictionary.json")
	if err != nil {
		return nil, fmt.Errorf("dictionary - %v", err)
	}
	err = json.Unmarshal(b, &results)
	if err != nil {
		return nil, fmt.Errorf("dictionary unmarshal - %v", err)
	}
	return &results, nil
}

func processCluster(db *sql.DB, cluster *Cluster, tables []*Table, tableFields map[string]string, fieldTypes map[string]string, ix int) (error, string) {

	out := ""
	for _, table := range tables {
		start := time.Now()
		out += fmt.Sprintln(" ", cluster.FacName, table.Name, "TABLE")
		rows, err, out1 := processData(db, cluster, tableFields, fieldTypes, table.Name, ix)
		out += out1
		if err != nil {
			out += fmt.Sprintln("Error=", err)
		}
		out += fmt.Sprintf("  %s %s Rows=%d, Duration=%v\n", cluster.FacName, table.Name, rows, time.Since(start))
	}

	return nil, out
}

func main() {
	concurrency := flag.Int("c", CONCURRENCY, "concurrency")
	flag.Parse()

	starttime := time.Now()

	dict, err := getDictionary()
	if err != nil {
		fmt.Println("Cannot load dictionary", err)
		return
	}

	psqlInfo := getPg()

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		fmt.Println("Error", err)
		return
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		fmt.Println("Cannot connect", psqlInfo, err)
		return
	}

	tableFields := map[string]string{}
	fieldTypes := map[string]string{}

	parseSchema(tableFields, fieldTypes, getRoot() + "schema.sql")

	workers := make(chan struct{}, *concurrency)

	time.Sleep(2 * time.Second)
	for ix, cluster := range dict.Clusters {

		workers <- struct{}{}
		go func(cluster *Cluster, items chan struct{}, ix int) {
			defer func() {
				<-items
			}()
			fmt.Printf("%d Cluster START %s - %s \n", ix, cluster.FacName, cluster.Address)
			err, out1 := processCluster(db, cluster, dict.Tables, tableFields, fieldTypes, ix)

			fmt.Println("------------------------------------------------------------------------------")
			fmt.Printf("%d Cluster REPORT %s - %s \n", ix, cluster.FacName, cluster.Address)
			fmt.Println("------------------------------------------------------------------------------")
			fmt.Println(out1)

			if err != nil {
				fmt.Println("Error processing", err)
			}
		}(cluster, workers, ix)
	}

	for i := 0; i < cap(workers); i++ {
		workers <- struct{}{}
	}

	fmt.Printf("Done. Duration=%v \n", time.Since(starttime))

}
