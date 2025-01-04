package db

import (
	"database/sql"
)

var db *sql.DB

func InitDB() {
	var err error
	db, err = sql.Open("sqlite3", "./backup.db")

	if err != nil {
		panic(err)
	}

	err = createFileRecordsTable()
	if err != nil {
		panic(err)
	}

	err = createAuthInfoTable()
	if err != nil {
		panic(err)
	}
}

func CloseDB() {
	db.Close()
}

func DB() *sql.DB {
	return db
}

func createFileRecordsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_records (
            path TEXT,
            mod_time DATETIME,
            backup_id TEXT,
            PRIMARY KEY (path, backup_id)
        )`)
	return err
}

func createAuthInfoTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS auth_info (
            access_token TEXT,
            refresh_token TEXT,
            expires_in INTEGER,
            user_id TEXT PRIMARY KEY
        )`)
	return err
}
