package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"timenotesblog/internal/auth"
	_ "modernc.org/sqlite"
)

func main() {
	root, _ := os.Getwd()
	// allow running from tools/resetpw or repo root
	candidates := []string{
		filepath.Join(root, "data", "blog.db"),
		filepath.Join(root, "..", "..", "data", "blog.db"),
	}
	dbPath := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			dbPath = c
			break
		}
	}
	if dbPath == "" {
		log.Fatal("blog.db not found")
	}
	hash, err := auth.HashPassword("123456", "")
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	res, err := db.Exec(`UPDATE users SET password_hash=?, credentials_version=credentials_version+1 WHERE username=?`, hash, "alg")
	if err != nil {
		log.Fatal(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("db=%s updated=%d user=alg password=123456\n", dbPath, n)
}
