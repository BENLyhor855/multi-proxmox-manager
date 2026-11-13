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
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if dsn == "" || email == "" || len(password) < 14 || len(password) > 1024 {
		log.Fatal("set MYSQL_DSN, BOOTSTRAP_ADMIN_EMAIL, and BOOTSTRAP_ADMIN_PASSWORD (14-1024 bytes)")
	}
	address, emailErr := mail.ParseAddress(email)
	if emailErr != nil || address.Address != email || len(email) > 254 {
		log.Fatal("a valid email address is required")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		log.Fatal(err)
	}
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	var locked int
	if err := db.QueryRowContext(ctx, `SELECT GET_LOCK('mmp.bootstrap.admin', 5)`).Scan(&locked); err != nil || locked != 1 {
		log.Fatal("administrator bootstrap is busy")
	}
	defer db.ExecContext(context.Background(), `SELECT RELEASE_LOCK('mmp.bootstrap.admin')`)
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&exists); err != nil {
		log.Fatal(err)
	}
	if exists > 0 {
		log.Fatal("a user with this email already exists; use a dedicated account-management workflow to change it")
	}
	var admins int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
		log.Fatal(err)
	}
	if admins > 0 {
		log.Fatal("an administrator already exists; bootstrap command is disabled after first admin creation")
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
	res, err := tx.ExecContext(ctx, `INSERT INTO users (email, password_hash, role, is_active) VALUES (?, ?, 'admin', TRUE)`, email, hash)
	if err != nil {
		log.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs (action,target_type,target_id,source_ip,details) VALUES ('user.create','user',?,'',JSON_OBJECT('role','admin','source','local-cli'))`, fmt.Sprint(id))
	if err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Administrator created: %s\n", email)
}
