package db

import (
	"fmt"
	"strings"
	"time"
)

// 文件记录结构
type FileRecord struct {
	ID       uint64    `db:"id"`        // 自增ID
	Path     string    `db:"path"`      // 文件路径
	ModTime  time.Time `db:"mod_time"`  // 文件修改时间
	BackupID string    `db:"backup_id"` // 备份ID
	Hash     string    `db:"hash"`      // 文件哈希值
}

// 保存文件记录到数据库
func SaveFileRecord(fr *FileRecord) error {
	// 注意：不包含ID字段，让数据库自动处理自增ID
	query := `INSERT INTO file_records (path, mod_time, backup_id, hash) 
              VALUES (?, ?, ?, ?)`
	_, err := db.Exec(query, fr.Path, fr.ModTime, fr.BackupID, fr.Hash)
	return err
}

// 批量保存文件记录
func BatchSaveFileRecords(records []*FileRecord) error {
	// 构建包含哈希字段的插入语句
	query := `INSERT INTO file_records (path, mod_time, backup_id, hash) VALUES `
	values := make([]string, len(records))

	for i, record := range records {
		// 转义字符串，防止SQL注入
		path := strings.ReplaceAll(record.Path, "'", "''")
		hash := strings.ReplaceAll(record.Hash, "'", "''")
		backupID := strings.ReplaceAll(record.BackupID, "'", "''")

		values[i] = fmt.Sprintf("('%s', '%s', '%s', '%s')",
			path,
			record.ModTime.Format("2006-01-02 15:04:05"),
			backupID,
			hash)
	}

	query += strings.Join(values, ",")
	_, err := db.Exec(query)
	return err
}

// 优化版本的批量保存（使用事务和预处理语句）
func BatchSaveFileRecordsOptimized(records []*FileRecord) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO file_records (path, mod_time, backup_id, hash) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, record := range records {
		_, err = stmt.Exec(record.Path, record.ModTime, record.BackupID, record.Hash)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// 删除文件记录
func DeleteFileRecord(backupId string) error {
	query := `DELETE FROM file_records WHERE backup_id = ?`
	_, err := db.Exec(query, backupId)
	return err
}

// 从数据库加载文件记录
func LoadFileRecords(backupId string) ([]*FileRecord, error) {
	// 修改查询以包含哈希字段
	query := `SELECT id, path, mod_time, backup_id, hash FROM file_records WHERE backup_id = ?`
	rows, err := db.Query(query, backupId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]*FileRecord, 0)
	for rows.Next() {
		record := &FileRecord{}
		// 更新Scan以包含ID和哈希字段
		err := rows.Scan(&record.ID, &record.Path, &record.ModTime, &record.BackupID, &record.Hash)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
