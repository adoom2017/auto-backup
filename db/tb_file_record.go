package db

import (
	"fmt"
	"strings"
	"time"
)

// 文件记录结构
type FileRecord struct {
	Path     string    `db:"path"`
	ModTime  time.Time `db:"mod_time"`
	BackupID string    `db:"backup_id"`
}

// 保存文件记录到数据库
func SaveFileRecord(fr *FileRecord) error {
	query := `INSERT INTO file_records (path, mod_time, backup_id) 
			  VALUES ($1, $2, $3)`
	_, err := db.Exec(query, fr.Path, fr.ModTime, fr.BackupID)
	return err
}

// 批量保存文件记录
func BatchSaveFileRecords(records []*FileRecord) error {
	query := `INSERT INTO file_records (path, mod_time, backup_id) VALUES `
	values := make([]string, len(records))
	for i, record := range records {
		values[i] = fmt.Sprintf("('%s', '%s', '%s')",
			record.Path,
			record.ModTime.Format("2006-01-02 15:04:05"),
			record.BackupID)
	}
	query += strings.Join(values, ",")
	_, err := db.Exec(query)
	return err
}

// 删除文件记录
func DeleteFileRecord(backupId string) error {
	query := `DELETE FROM file_records WHERE backup_id = ?`
	_, err := db.Exec(query, backupId)
	return err
}

// 从数据库加载文件记录
func LoadFileRecords(backupId string) ([]*FileRecord, error) {
	query := `SELECT path, mod_time, backup_id FROM file_records WHERE backup_id = ?`
	rows, err := db.Query(query, backupId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]*FileRecord, 0)
	for rows.Next() {
		record := &FileRecord{}
		err := rows.Scan(&record.Path, &record.ModTime, &record.BackupID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
