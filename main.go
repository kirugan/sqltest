package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"

	"time"
)

func main() {
	db, err := Open("postgres")
	if err != nil {
		panic(err)
	}

	rows, err := db.QueryContext(context.TODO(), "select pg_sleep(4)")
	if err != nil {
		panic(err)
	}

	for rows.Next() {
		var id []byte
		err := rows.Scan(&id)
		if err != nil {
			panic(err)
		}

		fmt.Println("Row", id)
	}

	time.Sleep(1 * time.Hour)

	fmt.Println("OK")
}

func Open(dbName string) (*sql.DB, error) {
	driverCfg := stdlib.DriverConfig{
		ConnConfig: pgx.ConnConfig{
			Host:     "127.0.0.1",
			Port:     6432,
			Database: dbName,
			User:     "postgres",
			Password: "example",
			RuntimeParams: map[string]string{
				"standard_conforming_strings": "on",
			},
			PreferSimpleProtocol: true,
		},
	}

	stdlib.RegisterDriverConfig(&driverCfg)

	db, err := sql.Open("pgx", driverCfg.ConnectionString(""))
	if err != nil {
		return nil, err
	}

	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(200)

	return db, nil
}