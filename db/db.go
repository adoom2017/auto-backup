package db

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

// 当前数据库架构版本
const CurrentSchemaVersion = 2

func InitDB() {
	var err error
	db, err = sql.Open("sqlite3", "./config/backup.db")

	if err != nil {
		panic(err)
	}

	// 创建并检查版本表
	err = createVersionTable()
	if err != nil {
		panic(err)
	}

	// 升级数据库
	err = upgradeSchema()
	if err != nil {
		panic(err)
	}

	// 创建必要的表
	err = createFileRecordsTable()
	if err != nil {
		panic(err)
	}

	err = createAuthInfoTable()
	if err != nil {
		panic(err)
	}
}

// CloseDB 关闭数据库连接
func CloseDB() {
	db.Close()
}

// DB 获取数据库连接
func DB() *sql.DB {
	return db
}

// 创建版本表
func createVersionTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS db_version (
        version INTEGER PRIMARY KEY,
        applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )`)

	if err != nil {
		return err
	}

	// 检查版本表是否有数据，如果没有则插入初始版本
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM db_version").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		_, err = db.Exec("INSERT INTO db_version (version) VALUES (1)")
		if err != nil {
			return err
		}
	}

	return nil
}

// 获取当前数据库版本
func getCurrentVersion() (int, error) {
	var version int
	err := db.QueryRow("SELECT MAX(version) FROM db_version").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// 更新数据库版本
func updateVersion(version int) error {
	_, err := db.Exec("INSERT INTO db_version (version) VALUES (?)", version)
	return err
}

// upgradeSchema 处理数据库升级
func upgradeSchema() error {
	currentVersion, err := getCurrentVersion()
	if err != nil {
		return err
	}

	if currentVersion < CurrentSchemaVersion {
		log.Printf("需要升级数据库从版本 %d 到 %d", currentVersion, CurrentSchemaVersion)

		// 开始事务进行升级
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		// 按顺序执行升级脚本
		for version := currentVersion + 1; version <= CurrentSchemaVersion; version++ {
			log.Printf("应用升级脚本到版本 %d", version)

			err = applyUpgradeScript(tx, version)
			if err != nil {
				log.Printf("升级到版本 %d 失败: %v", version, err)
				return err
			}

			// 更新版本号
			_, err = tx.Exec("INSERT INTO db_version (version) VALUES (?)", version)
			if err != nil {
				return err
			}

			log.Printf("成功升级到版本 %d", version)
		}

		// 提交事务
		err = tx.Commit()
		if err != nil {
			return err
		}

		log.Printf("数据库升级完成，当前版本: %d", CurrentSchemaVersion)
	} else {
		log.Printf("数据库已是最新版本: %d", currentVersion)
	}

	return nil
}

// applyUpgradeScript 应用指定版本的升级脚本
func applyUpgradeScript(tx *sql.Tx, version int) error {
	switch version {
	case 2:
		// 版本2：添加hash字段到file_records表
		_, err := tx.Exec(`ALTER TABLE file_records ADD COLUMN hash TEXT`)
		if err != nil {
			// 如果列已存在，SQLite会返回错误，这里我们忽略这种错误
			// 注意：这是SQLite特有的行为
			log.Printf("添加hash列时出现错误(可能列已存在): %v", err)
			return nil
		}
		return nil
	// 添加更多版本升级脚本
	case 3:
		// 版本3的升级脚本
		// ...
		return nil
	default:
		log.Printf("没有找到版本 %d 的升级脚本", version)
		return nil
	}
}

// 原有的表创建函数
func createFileRecordsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_records (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        path TEXT,
        mod_time DATETIME,
        backup_id TEXT,
        is_dir INTEGER,
        hash TEXT
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
