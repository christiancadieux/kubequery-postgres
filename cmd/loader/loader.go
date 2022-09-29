package main

import (
	"database/sql"
	"flag"
	"fmt"
	dict "github.com/christiancadieux/kubequery-postgres/pkg/dictionary"
	processor "github.com/christiancadieux/kubequery-postgres/pkg/processor"
	_ "github.com/lib/pq"
	"log"
	"os"
	"strconv"
	"time"
)

const (
	CONCURRENCY = 1
)

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
	return path + "/"
}

func main() {
	concurrency := flag.Int("c", CONCURRENCY, "concurrency")
	flag.Parse()

	starttime := time.Now()

	dictio, err := dict.NewDictionary(getRoot())
	if err != nil {
		log.Fatalf("Cannot load dictionary - %v", err)
	}

	psqlInfo := getPg()

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatalf("Cannot connect to postgres - %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("Cannot ping", psqlInfo, err)
	}

	processor, err := processor.NewProcessor(db, dictio, getRoot(), *concurrency)
	if err != nil {
		log.Fatalf("NewProcessor error - %v", err)
	}

	err = dictio.ParseSchema(getRoot() + "schema.sql")
	if err != nil {
		log.Fatalf("ParseSchema error - %v", err)
	}

	time.Sleep(2 * time.Second)

	processor.Run()

	fmt.Printf("Done. Duration=%v \n", time.Since(starttime))

}
