package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"timenotesblog/internal/auth"
	_ "modernc.org/sqlite"
)

func main() {
	userFlag := flag.String("user", "alg", "要重置密码的用户名")
	configFlag := flag.String("config", "", "config.json 路径（用于读取 passwordPepper）")
	flag.Parse()

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

	pepper := loadPepper(*configFlag, root)

	fmt.Printf("为用户 %s 设置新密码（输入不回显）：\n", *userFlag)
	secret, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil || len(secret) == 0 {
		log.Fatal("读取密码失败")
	}
	if len(secret) < 8 {
		log.Fatal("密码至少 8 位")
	}
	fmt.Println("再输入一次确认：")
	confirm, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil || string(confirm) != string(secret) {
		log.Fatal("两次输入不一致")
	}

	hash, err := auth.HashPassword(string(secret), pepper)
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	// must_change_credentials 置 1：工具重置的是应急密码，首次登录仍强制走凭据迁移。
	res, err := db.Exec(
		`UPDATE users SET password_hash=?, credentials_version=credentials_version+1, must_change_credentials=1 WHERE username=?`,
		hash, *userFlag,
	)
	if err != nil {
		log.Fatal(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("db=%s updated=%d user=%s\n", dbPath, n, *userFlag)
}

// loadPepper 读取 config.json 的 passwordPepper，保证哈希口径与服务端一致。
func loadPepper(configFlag, root string) string {
	path := strings.TrimSpace(configFlag)
	if path == "" {
		path = filepath.Join(root, "config.json")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		PasswordPepper string `json:"passwordPepper"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return ""
	}
	return cfg.PasswordPepper
}
