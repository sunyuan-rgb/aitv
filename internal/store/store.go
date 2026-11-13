// Package store 负责 SQLite 连接、建表与预置数据。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 包装 *sql.DB。
type Store struct {
	DB *sql.DB
}

// Open 打开 SQLite。单连接串行化事务，避免并发写锁冲突。
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 单写者模型，教学规模足够且避免 SQLITE_BUSY
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close 关闭连接。
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			role TEXT NOT NULL,
			balance INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS episodes(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			seq INTEGER NOT NULL,
			status TEXT NOT NULL,
			cur_title TEXT NOT NULL,
			cur_is_free INTEGER NOT NULL,
			cur_price INTEGER NOT NULL DEFAULT 0,
			cur_play_url TEXT NOT NULL,
			pub_title TEXT NOT NULL DEFAULT '',
			pub_is_free INTEGER NOT NULL DEFAULT 0,
			pub_price INTEGER NOT NULL DEFAULT 0,
			pub_play_url TEXT NOT NULL DEFAULT '',
			submitted_by TEXT NOT NULL DEFAULT '',
			reject_reason TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS revisions(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			episode_id INTEGER NOT NULL REFERENCES episodes(id),
			title TEXT NOT NULL,
			is_free INTEGER NOT NULL,
			play_url TEXT NOT NULL,
			status TEXT NOT NULL,
			reject_reason TEXT NOT NULL DEFAULT '',
			submitted_by TEXT NOT NULL DEFAULT '',
			reviewed_by TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS purchases(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT UNIQUE,
			user_id INTEGER NOT NULL REFERENCES users(id),
			episode_id INTEGER NOT NULL REFERENCES episodes(id),
			price INTEGER NOT NULL,
			created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS entitlements(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			episode_id INTEGER NOT NULL,
			purchase_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			refunded_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS refunds(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT UNIQUE,
			user_id INTEGER NOT NULL,
			episode_id INTEGER NOT NULL,
			entitlement_id INTEGER NOT NULL,
			amount INTEGER NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			reject_reason TEXT NOT NULL DEFAULT '',
			processed_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS op_logs(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			episode_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			actor TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_ent_user_ep ON entitlements(user_id, episode_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ref_user_ep ON refunds(user_id, episode_id)`,
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

// Seed 预置题目要求的账号与一部 6 集短剧（前2集免费，后4集各100平台币，均为草稿态待编辑提交审核）。
func (s *Store) Seed() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	users := []struct {
		name, pass, role string
		bal              int64
	}{
		{"editor", "pass123", "editor", 0},
		{"reviewer", "pass123", "reviewer", 0},
		{"ops", "pass123", "ops", 0},
		{"support", "pass123", "support", 0},
		{"u1", "pass123", "user", 500},
		{"u2", "pass123", "user", 300},
		{"u3", "pass123", "user", 100}, // 恰好够买一集，用于并发争抢演示
	}
	for _, u := range users {
		if _, err := s.DB.Exec(`INSERT INTO users(username,password,role,balance) VALUES(?,?,?,?)`,
			u.name, u.pass, u.role, u.bal); err != nil {
			return err
		}
	}
	for seq := 1; seq <= 6; seq++ {
		free := 0
		price := int64(100)
		if seq <= 2 {
			free = 1
			price = 0
		}
		title := fmt.Sprintf("《霸总爱上我》第%d集", seq)
		url := fmt.Sprintf("/static/demo/ep%d.mp4", seq)
		if _, err := s.DB.Exec(`INSERT INTO episodes(seq,status,cur_title,cur_is_free,cur_price,cur_play_url) VALUES(?,?,?,?,?,?)`,
			seq, "draft", title, free, price, url); err != nil {
			return err
		}
	}
	return nil
}
