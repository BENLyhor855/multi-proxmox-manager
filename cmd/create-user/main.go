package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"example.com/multi-proxmox-manager/internal/security"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	email := strings.ToLower(strings.TrimSpace(os.Getenv("NEW_USER_EMAIL")))
	role := strings.TrimSpace(os.Getenv("NEW_USER_ROLE"))
	password := os.Getenv("NEW_USER_PASSWORD")
	if dsn == "" || email == "" || (role != "operator" && role != "viewer") || len(password) < 14 || len(password) > 1024 {
		log.Fatal("set MYSQL_DSN, NEW_USER_EMAIL, NEW_USER_ROLE (operator|viewer), and NEW_USER_PASSWORD (14-1024 bytes)")
	}
	address, emailErr := mail.ParseAddress(email)
	if emailErr != nil || address.Address != email || len(email) > 254 {
		log.Fatal("a valid email address is required")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		log.Fatal(err)
	}
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 5*time.Second, 5*time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	var adminCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = TRUE`).Scan(&adminCount); err != nil {
		log.Fatal(err)
	}
	if adminCount == 0 {
		log.Fatal("create the first administrator before adding other users")
	}
	var userCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&userCount); err != nil {
		log.Fatal(err)
	}
	if userCount != 0 {
		log.Fatal("a user with this email already exists")
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (email, password_hash, role, is_active) VALUES (?, ?, ?, TRUE)`, email, hash, role)
	if err != nil {
		log.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs (action,target_type,target_id,source_ip,details) VALUES ('user.create','user',?,'',JSON_OBJECT('role',?,'source','local-cli'))`, fmt.Sprint(id), role)
	if err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("User created: %s (%s)\n", email, role)
}
