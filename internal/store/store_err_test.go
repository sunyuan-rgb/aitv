package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCorruptedDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.db")
	// 写入非 SQLite 内容 → Ping 阶段失败
	if err := os.WriteFile(path, []byte("this is not a sqlite database file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("损坏文件应导致 Open 失败")
	}
}
