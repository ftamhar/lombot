package database

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func OpenDbConnection() (*sql.DB, error) {
	log.Println("=> open db connection")

	f, err := os.OpenFile("foo.sqlite", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	f.Close()

	dbConnString := "file:foo.sqlite?cache=shared&mode=rwc"

	db, err := sql.Open("sqlite3", dbConnString)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(" CREATE TABLE IF NOT EXISTS `subscriptions` ( `room_id` int8, `user_name` varchar(100), PRIMARY KEY (`room_id`, `user_name`))")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(4)

	return db, nil
}
