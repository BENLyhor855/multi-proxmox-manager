package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/multi-proxmox-manager/internal/config"
	"example.com/multi-proxmox-manager/internal/database"
	"example.com/multi-proxmox-manager/internal/proxmox"
	"example.com/multi-proxmox-manager/internal/security"
	"example.com/multi-proxmox-manager/internal/testutil"
)

// TEST_MYSQL_DSN must name a disposable database: this test clears its tables.
func TestMySQLPhase1(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to a disposable MySQL 8 database")
	}
	db, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"001_init.sql", "002_sessions.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range strings.Split(string(data), ";") {
			if strings.TrimSpace(q) != "" {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, table := range []string{"sessions", "audit_logs", "proxmox_servers", "users"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}
	password := "integration-password-2026"
	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "operator", "viewer"} {
		if _, err := db.Exec(`INSERT INTO users (email,password_hash,role) VALUES (?,?,?)`, role+"@example.com", hash, role); err != nil {
			t.Fatal(err)
		}
	}
	one, ca1 := testutil.PVE(t, "127.0.0.4", "installation-secret-a", "8.4")
	two, ca2 := testutil.PVE(t, "127.0.0.5", "installation-secret-b", "9.0")
	_, cidr, _ := net.ParseCIDR("127.0.0.0/8")
	pve, err := proxmox.NewClient([]*net.IPNet{cidr}, testutil.CAFile(t, ca1, ca2))
	if err != nil {
		t.Fatal(err)
	}
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{42}, 32))
	cfg := config.Config{JWTSecret: bytes.Repeat([]byte{24}, 48), CookieSecure: true}
	app := New(db, cfg, box, pve)
	handler := app.Handler()
	call := func(method, path string, body any, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "https://manager.test"+path, bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	mustStatus := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d: %s", status, w.Code, w.Body.String())
		}
	}
	login := func(role string) ([]*http.Cookie, string) {
		t.Helper()
		w := call("POST", "/api/v1/auth/login", map[string]string{"email": role + "@example.com", "password": password}, nil, "")
		mustStatus(w, 200)
		cookies := w.Result().Cookies()
		var csrf string
		for _, c := range cookies {
			if !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatal("unsafe cookie")
			}
			if c.Name == csrfCookie {
				csrf = c.Value
			}
			if c.Name == sessionCookie && !c.HttpOnly {
				t.Fatal("session readable by JS")
			}
		}
		if csrf == "" {
			t.Fatal("no CSRF cookie")
		}
		return cookies, csrf
	}
	mustStatus(call("GET", "/api/v1/proxmox/servers", nil, nil, ""), 401)
	admin, csrf := login("admin")
	operator, opCSRF := login("operator")
	viewer, viewCSRF := login("viewer")
	mustStatus(call("POST", "/api/v1/proxmox/servers", map[string]string{}, admin, ""), 403)
	forged := []*http.Cookie{}
	for _, c := range admin {
		copy := *c
		if copy.Name == csrfCookie {
			copy.Value = "attacker"
		}
		forged = append(forged, &copy)
	}
	mustStatus(call("POST", "/api/v1/auth/logout", map[string]string{}, forged, "attacker"), 403)
	mustStatus(call("POST", "/api/v1/proxmox/servers", map[string]string{}, operator, opCSRF), 403)
	mustStatus(call("GET", "/api/v1/audit-logs", nil, viewer, ""), 403)
	ids := []string{}
	for i, f := range []testutil.Fixture{one, two} {
		w := call("POST", "/api/v1/proxmox/servers", map[string]string{"name": fmt.Sprintf("PVE-%d", i), "base_url": f.URL, "token_id": f.TokenID, "token_secret": f.Secret}, admin, csrf)
		mustStatus(w, 201)
		if strings.Contains(w.Body.String(), f.Secret) {
			t.Fatal("credential returned")
		}
		var got struct{ Item serverRecord }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Item.Status != "connected" || got.Item.Version != f.Version || got.Item.LastCheckedAt == nil {
			t.Fatalf("bad registration: %s", w.Body.String())
		}
		ids = append(ids, got.Item.ID)
		var encrypted string
		if err := db.QueryRow(`SELECT token_secret_enc FROM proxmox_servers WHERE id=?`, got.Item.ID).Scan(&encrypted); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(encrypted, f.Secret) {
			t.Fatal("plaintext stored")
		}
		plain, err := box.Open(encrypted)
		if err != nil || plain != f.Secret {
			t.Fatal("encryption round trip failed")
		}
	}
	mustStatus(call("POST", "/api/v1/proxmox/servers/"+ids[0]+"/test", map[string]string{}, viewer, viewCSRF), 403)
	mustStatus(call("POST", "/api/v1/proxmox/servers/"+ids[1]+"/test", map[string]string{}, operator, opCSRF), 200)
	mustStatus(call("DELETE", "/api/v1/proxmox/servers/"+ids[0], nil, operator, opCSRF), 403)
	mustStatus(call("GET", "/api/v1/proxmox/servers", nil, viewer, ""), 200)
	// Re-read roles on every request instead of trusting the old JWT claim.
	db.Exec(`UPDATE users SET role='viewer' WHERE email='operator@example.com'`)
	mustStatus(call("POST", "/api/v1/proxmox/servers/"+ids[1]+"/test", map[string]string{}, operator, opCSRF), 403)
	db.Exec(`UPDATE users SET is_active=FALSE WHERE email='viewer@example.com'`)
	mustStatus(call("GET", "/api/v1/auth/me", nil, viewer, ""), 401)
	mustStatus(call("POST", "/api/v1/proxmox/servers", map[string]string{"name": "PVE-duplicate", "base_url": two.URL, "token_id": two.TokenID, "token_secret": two.Secret}, admin, csrf), 409)
	// Simulate a missing audit table; a deletion must roll back.
	if _, err := db.Exec(`RENAME TABLE audit_logs TO audit_logs_offline`); err != nil {
		t.Fatal(err)
	}
	mustStatus(call("DELETE", "/api/v1/proxmox/servers/"+ids[0], nil, admin, csrf), 503)
	mustStatus(call("POST", "/api/v1/proxmox/servers", map[string]string{"name": "Must rollback", "base_url": "https://127.0.0.6:8006", "token_id": one.TokenID, "token_secret": one.Secret}, admin, csrf), 503)
	var unaudited int
	if err := db.QueryRow(`SELECT COUNT(*) FROM proxmox_servers WHERE name='Must rollback'`).Scan(&unaudited); err != nil || unaudited != 0 {
		t.Fatal("unaudited registration persisted", err)
	}

	if _, err := db.Exec(`RENAME TABLE audit_logs_offline TO audit_logs`); err != nil {
		t.Fatal(err)
	}
	var exists int
	db.QueryRow(`SELECT COUNT(*) FROM proxmox_servers WHERE id=?`, ids[0]).Scan(&exists)
	if exists != 1 {
		t.Fatal("unaudited deletion committed")
	}
	mustStatus(call("DELETE", "/api/v1/proxmox/servers/"+ids[0], nil, admin, csrf), 200)
	audit := call("GET", "/api/v1/audit-logs", nil, admin, "")
	mustStatus(audit, 200)
	for _, action := range []string{"auth.login", "proxmox.server.create", "proxmox.server.test", "proxmox.server.delete"} {
		if !strings.Contains(audit.Body.String(), action) {
			t.Fatalf("missing audit %s", action)
		}
	}
	mustStatus(call("POST", "/api/v1/auth/logout", map[string]string{}, admin, csrf), 200)
	mustStatus(call("GET", "/api/v1/auth/me", nil, admin, ""), 401)
	// A fresh handler resets only the IP limiter, never the durable account lockout.
	handler = app.Handler()
	for i := 0; i < 8; i++ {
		mustStatus(call("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong-password"}, nil, ""), 401)
	}
	var until sql.NullTime
	db.QueryRow(`SELECT locked_until FROM users WHERE email='admin@example.com'`).Scan(&until)
	if !until.Valid || !until.Time.After(time.Now()) {
		t.Fatal("lockout not enforced")
	}
	mustStatus(call("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": password}, nil, ""), 401)
	db.Exec(`UPDATE users SET locked_until=NULL,failed_login_count=0 WHERE email='admin@example.com'`)
	handler = app.Handler()
	admin, csrf = login("admin")
	db.Exec(`UPDATE sessions SET expires_at=UTC_TIMESTAMP()-INTERVAL 1 SECOND`)
	mustStatus(call("GET", "/api/v1/auth/me", nil, admin, ""), 401)
}
