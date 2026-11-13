package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/multi-proxmox-manager/internal/security"
)

const (
	sessionCookie = "mmp_session"
	csrfCookie    = "mmp_csrf"
	sessionTTL    = 15 * time.Minute
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authUser struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	Active       bool
	LockedUntil  sql.NullTime
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if len(email) < 5 || len(email) > 254 || len(req.Password) == 0 || len(req.Password) > 1024 {
		writeError(w, 401, "invalid email or password")
		return
	}
	ctx, cancel := contextTimeout(r, 8*time.Second)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, 503, "authentication unavailable")
		return
	}
	defer tx.Rollback()
	var u authUser
	err = tx.QueryRowContext(ctx, `SELECT id, email, password_hash, role, is_active, locked_until FROM users WHERE email = ? LIMIT 1 FOR UPDATE`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Active, &u.LockedUntil)
	if err != nil && err != sql.ErrNoRows {
		writeError(w, 503, "authentication unavailable")
		return
	}
	if err == sql.ErrNoRows || !u.Active || (u.LockedUntil.Valid && u.LockedUntil.Time.After(time.Now().UTC())) {
		tx.Rollback()
		security.VerifyPassword("$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", req.Password)
		a.audit(r, u.ID, "auth.login.failed", "user", "", nil)
		writeError(w, 401, "invalid email or password")
		return
	}
	if !security.VerifyPassword(u.PasswordHash, req.Password) {
		_, err = tx.ExecContext(ctx, `UPDATE users SET locked_until = CASE WHEN failed_login_count + 1 >= 8 THEN UTC_TIMESTAMP() + INTERVAL 15 MINUTE ELSE locked_until END, failed_login_count = CASE WHEN failed_login_count + 1 >= 8 THEN 0 ELSE failed_login_count + 1 END WHERE id = ?`, u.ID)
		if err == nil {
			err = auditWrite(ctx, tx, r, u.ID, "auth.login.failed", "user", strconv.FormatInt(u.ID, 10), nil)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeError(w, 503, "authentication unavailable")
			return
		}
		writeError(w, 401, "invalid email or password")
		return
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET failed_login_count = 0, locked_until = NULL, last_login_at = UTC_TIMESTAMP() WHERE id = ?`, u.ID)
	if err != nil {
		writeError(w, 503, "authentication unavailable")
		return
	}
	token, err := security.NewSession(a.cfg.JWTSecret, strconv.FormatInt(u.ID, 10), u.Email, u.Role, sessionTTL)
	if err != nil {
		writeError(w, 500, "unable to create session")
		return
	}
	csrf, err := security.RandomToken(32)
	if err != nil {
		writeError(w, 500, "unable to create session")
		return
	}
	// Store only bearer/CSRF hashes, with bounded opportunistic expiry cleanup.
	_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= UTC_TIMESTAMP() LIMIT 1000`)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, csrf_hash, expires_at) VALUES (?, ?, ?, ?)`, tokenHash(token), u.ID, tokenHash(csrf), time.Now().UTC().Add(sessionTTL))
	}
	if err == nil {
		err = auditWrite(ctx, tx, r, u.ID, "auth.login", "user", strconv.FormatInt(u.ID, 10), nil)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 503, "authentication unavailable")
		return
	}
	a.setSessionCookies(w, token, csrf, int(sessionTTL.Seconds()))
	writeJSON(w, 200, map[string]any{"user": Principal{ID: u.ID, Email: u.Email, Role: u.Role}})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": principal(r)})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	auditID := strconv.FormatInt(p.ID, 10)
	cookie, _ := r.Cookie(sessionCookie)
	ctx, cancel := contextTimeout(r, 3*time.Second)
	defer cancel()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, 503, "logout unavailable")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash(cookie.Value))
	if err == nil {
		err = auditWrite(ctx, tx, r, p.ID, "auth.logout", "user", auditID, nil)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 503, "logout unavailable")
		return
	}
	a.setSessionCookies(w, "", "", -1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) setSessionCookies(w http.ResponseWriter, session, csrf string, maxAge int) {
	secure := a.cfg.CookieSecure
	base := http.Cookie{Path: "/", Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
	sc := base
	sc.Name, sc.Value, sc.HttpOnly = sessionCookie, session, true
	w.Header().Add("Set-Cookie", sc.String())
	cc := base
	cc.Name, cc.Value, cc.HttpOnly = csrfCookie, csrf, false
	w.Header().Add("Set-Cookie", cc.String())
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		claims, err := security.ParseSession(a.cfg.JWTSecret, cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired; sign in again")
			return
		}
		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		var p Principal
		var active bool
		var csrfHash string
		ctx, cancel := contextTimeout(r, 3*time.Second)
		defer cancel()
		err = a.db.QueryRowContext(ctx, `SELECT u.id, u.email, u.role, u.is_active, s.csrf_hash FROM users u JOIN sessions s ON s.user_id = u.id WHERE u.id = ? AND s.token_hash = ? AND s.expires_at > UTC_TIMESTAMP()`, id, tokenHash(cookie.Value)).Scan(&p.ID, &p.Email, &p.Role, &active, &csrfHash)
		if err != nil || !active {
			writeError(w, http.StatusUnauthorized, "account unavailable")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			csrfCookieValue, cErr := r.Cookie(csrfCookie)
			header := r.Header.Get("X-CSRF-Token")
			if cErr != nil || header == "" || (!hmac.Equal([]byte(csrfCookieValue.Value), []byte(header)) || !hmac.Equal([]byte(tokenHash(header)), []byte(csrfHash))) {
				writeError(w, http.StatusForbidden, "CSRF validation failed")
				return
			}
		}
		ctxWithUser := context.WithValue(r.Context(), userContextKey{}, p)
		next.ServeHTTP(w, r.WithContext(ctxWithUser))
	})
}

func (a *App) requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principal(r)
			for _, role := range roles {
				if p.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "insufficient permissions")
		}))
	}
}
