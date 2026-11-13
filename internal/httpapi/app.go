package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"example.com/multi-proxmox-manager/internal/config"
	"example.com/multi-proxmox-manager/internal/proxmox"
	"example.com/multi-proxmox-manager/internal/security"
)

type App struct {
	db  *sql.DB
	cfg config.Config
	box *security.SecretBox
	pve *proxmox.Client
}

func New(db *sql.DB, cfg config.Config, box *security.SecretBox, pve *proxmox.Client) *App {
	return &App{db: db, cfg: cfg, box: box, pve: pve}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", a.health)
	mux.Handle("POST /api/v1/auth/login", loginThrottle(http.HandlerFunc(a.login)))
	mux.Handle("GET /api/v1/auth/me", a.requireAuth(http.HandlerFunc(a.me)))
	mux.Handle("POST /api/v1/auth/logout", a.requireAuth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /api/v1/proxmox/servers", a.requireAuth(http.HandlerFunc(a.listServers)))
	mux.Handle("GET /api/v1/audit-logs", a.requireRole("admin")(http.HandlerFunc(a.listAuditLogs)))
	mux.Handle("POST /api/v1/proxmox/servers", a.requireRole("admin")(http.HandlerFunc(a.createServer)))
	mux.Handle("POST /api/v1/proxmox/servers/{id}/test", a.requireRole("admin", "operator")(http.HandlerFunc(a.testServer)))
	mux.Handle("DELETE /api/v1/proxmox/servers/{id}", a.requireRole("admin")(http.HandlerFunc(a.deleteServer)))

	return securityHeaders(sameOrigin(a.cfg.PublicOrigin, mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextTimeout(r, 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
