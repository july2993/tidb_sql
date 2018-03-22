package main

import (
	"database/sql"
	"fmt"
	"strconv"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "root:@tcp(127.0.0.1:4000)/test")
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()

	_, err = db.Query("drop database if exists test_sql")
	if err != nil {
		panic(err.Error())
	}
	_, err = db.Query("create database test_sql")
	if err != nil {
		panic(err.Error())
	}

	_, err = db.Query("create table test_sql.test(a int, b varchar(255))")
	if err != nil {
		panic(err.Error())
	}

	_, err = db.Query("insert into test_sql.test values( ?, ? )", 1024, "1024")
	if err != nil {
		panic(err.Error())
	}

	// test Null
	_, err = db.Query("insert into test_sql.test values( ?, ? )", nil, nil)
	if err != nil {
		panic(err.Error())
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(v int) {
			// use COM_STMT_**
			_, err = db.Query("insert into test_sql.test values( ?, ? )", v, strconv.Itoa(v))
			if err != nil {
				panic(err.Error())
			}

			// use COM_QUERY
			_, err = db.Query(fmt.Sprintf("insert into test_sql.test values( %v, '%v' )", v, v))
			if err != nil {
				panic(err.Error())
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

}
