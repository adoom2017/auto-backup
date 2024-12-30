package db

import (
	"auto-backup/config"
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

// 保存认证信息到数据库
func SaveAuthInfo(a *config.AuthInfo) error {

	query := `INSERT INTO auth_info (access_token, refresh_token, expires_in, user_id) 
			  VALUES ($1, $2, $3, $4)
			  ON CONFLICT (user_id) DO UPDATE 
			  SET access_token = $1, refresh_token = $2, expires_in = $3`

	_, err := db.Exec(query, a.AccessToken, a.RefreshToken, a.ExpiresIn, a.UserID)
	return err
}

// 从数据库加载认证信息
func LoadAuthInfo() (*config.AuthInfo, error) {
	a := &config.AuthInfo{}

	query := `SELECT user_id, access_token, refresh_token, expires_in 
			  FROM auth_info LIMIT 1`

	err := db.QueryRow(query).Scan(&a.UserID, &a.AccessToken, &a.RefreshToken, &a.ExpiresIn)
	if err != nil {
		return nil, err
	}
	return a, nil
}
