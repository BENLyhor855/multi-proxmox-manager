package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/multi-proxmox-manager/internal/proxmox"
	"github.com/go-sql-driver/mysql"
)

type serverRecord struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	BaseURL       string     `json:"base_url"`
	TokenID       string     `json:"token_id"`
	Location      string     `json:"location"`
	Notes         string     `json:"notes"`
	Status        string     `json:"status"`
	Version       string     `json:"version"`
	NodeCount     int        `json:"node_count"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastError     string     `json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
}

type createServerRequest struct {
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	TokenID     string `json:"token_id"`
	TokenSecret string `json:"token_secret"`
	Location    string `json:"location"`
	Notes       string `json:"notes"`
}

var tokenIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+![A-Za-z0-9_.-]+$`)
var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func (a *App) listServers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextTimeout(r, 5*time.Second)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT id, name, base_url, token_id, location, notes, status,
		COALESCE(version, ''), node_count, last_checked_at, COALESCE(last_error, ''), created_at
		FROM proxmox_servers ORDER BY name ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load Proxmox installations")
		return
	}
	defer rows.Close()
	servers := make([]serverRecord, 0)
	for rows.Next() {
		var s serverRecord
		var checked sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &s.BaseURL, &s.TokenID, &s.Location, &s.Notes, &s.Status,
			&s.Version, &s.NodeCount, &checked, &s.LastError, &s.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "unable to read Proxmox installations")
			return
		}
		if checked.Valid {
			t := checked.Time
			s.LastCheckedAt = &t
		}
		servers = append(servers, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to read Proxmox installations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": servers})
}

func (a *App) createServer(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.TokenID = strings.TrimSpace(req.TokenID)
	req.Location = strings.TrimSpace(req.Location)
	req.Notes = strings.TrimSpace(req.Notes)
	baseURL, err := proxmox.ValidateBaseURL(req.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Name) < 2 || len(req.Name) > 80 {
		writeError(w, http.StatusBadRequest, "name must be 2 to 80 characters")
		return
	}
	if !tokenIDPattern.MatchString(req.TokenID) || len(req.TokenID) > 255 {
		writeError(w, http.StatusBadRequest, "token_id must use the Proxmox format user@realm!tokenid")
		return
	}
	if req.TokenSecret == "" || len(req.TokenSecret) > 512 || strings.ContainsAny(req.TokenSecret, "\r\n") {
		writeError(w, http.StatusBadRequest, "token_secret is required and must be valid")
		return
	}
	if len(req.Location) > 120 || len(req.Notes) > 500 {
		writeError(w, http.StatusBadRequest, "location or notes is too long")
		return
	}
	secretEncrypted, err := a.box.Seal(req.TokenSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to protect API credential")
		return
	}
	id, err := newUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create Proxmox installation ID")
		return
	}
	p := principal(r)
	ctx, cancel := contextTimeout(r, 5*time.Second)
	defer cancel()
	err = a.withAudit(ctx, r, p.ID, "proxmox.server.create", "proxmox_server", id, map[string]string{"name": req.Name, "base_url": baseURL}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO proxmox_servers
		(id, name, base_url, token_id, token_secret_enc, location, notes, status, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'unknown', ?)`, id, req.Name, baseURL, req.TokenID,
			secretEncrypted, req.Location, req.Notes, p.ID)
		return err
	})
	if err != nil {
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			writeError(w, http.StatusConflict, "installation name or URL already registered")
		} else {
			writeError(w, http.StatusServiceUnavailable, "unable to register installation; database or audit unavailable")
		}
		return
	}

	// A failed first connection does not discard the registration. The operator can fix
	// routing, firewall or CA trust and run the connection test again.
	result, testErr := a.runServerTest(r.Context(), baseURL, req.TokenID, req.TokenSecret)
	status, version, nodeCount, lastError := "connected", result.Version, result.NodeCount, ""
	if testErr != nil {
		status, lastError = "disconnected", safeError(testErr)
	}
	updateCtx, updateCancel := context.WithTimeout(r.Context(), 3*time.Second)
	updateErr := a.saveTest(updateCtx, r, p.ID, id, status, version, nodeCount, lastError)
	updateCancel()
	if updateErr != nil {
		writeError(w, 503, "installation registered but connection status could not be saved; refresh and retry test")
		return
	}
	checked := time.Now().UTC()
	response := serverRecord{ID: id, Name: req.Name, BaseURL: baseURL, TokenID: req.TokenID,
		Location: req.Location, Notes: req.Notes, Status: status, Version: version,
		NodeCount: nodeCount, LastError: lastError, LastCheckedAt: &checked, CreatedAt: time.Now().UTC()}
	writeJSON(w, http.StatusCreated, map[string]any{"item": response, "connection": result})
}

func (a *App) testServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid installation ID")
		return
	}
	var baseURL, tokenID, encrypted string
	ctx, cancel := contextTimeout(r, 3*time.Second)
	err := a.db.QueryRowContext(ctx, `SELECT base_url, token_id, token_secret_enc FROM proxmox_servers WHERE id = ?`, id).
		Scan(&baseURL, &tokenID, &encrypted)
	cancel()
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Proxmox installation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load Proxmox credentials")
		return
	}
	secret, err := a.box.Open(encrypted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to decrypt stored Proxmox credential; rotate and re-enter the API token")
		return
	}
	p := principal(r)
	result, testErr := a.runServerTest(r.Context(), baseURL, tokenID, secret)
	status, version, nodeCount, lastError := "connected", result.Version, result.NodeCount, ""
	if testErr != nil {
		status, lastError = "disconnected", safeError(testErr)
	}
	updateCtx, updateCancel := context.WithTimeout(r.Context(), 3*time.Second)
	updateErr := a.saveTest(updateCtx, r, p.ID, id, status, version, nodeCount, lastError)
	updateCancel()
	if updateErr != nil {
		writeError(w, http.StatusInternalServerError, "connection test completed but status could not be saved")
		return
	}

	if testErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"status": status, "error": lastError})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "connection": result})
}

func (a *App) deleteServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid installation ID")
		return
	}
	p := principal(r)
	ctx, cancel := contextTimeout(r, 3*time.Second)
	defer cancel()
	err := a.withAudit(ctx, r, p.ID, "proxmox.server.delete", "proxmox_server", id, nil, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM proxmox_servers WHERE id = ?`, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Proxmox installation not found")
		return
	}
	if err != nil {
		writeError(w, 503, "unable to remove Proxmox installation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (a *App) runServerTest(parent context.Context, baseURL, tokenID, secret string) (proxmox.TestResult, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	return a.pve.Test(ctx, baseURL, tokenID, secret)
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func auditWrite(ctx context.Context, db executor, r *http.Request, userID int64, action, targetType, targetID string, details any) error {
	var actor any
	if userID != 0 {
		actor = userID
	}
	var payload []byte
	if details != nil {
		var err error
		payload, err = json.Marshal(details)
		if err != nil {
			return err
		}
	}
	_, err := db.ExecContext(ctx, `INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, source_ip, details) VALUES (?, ?, ?, ?, ?, ?)`, actor, action, targetType, targetID, sourceIP(r), nullableJSON(payload))
	return err
}

func (a *App) audit(r *http.Request, userID int64, action, targetType, targetID string, details any) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := auditWrite(ctx, a.db, r, userID, action, targetType, targetID, details); err != nil {
		slog.Error("audit event write failed", "action", action, "error", err)
	}
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	v := strings.TrimSpace(err.Error())
	if len(v) > 450 {
		v = v[:450]
	}
	return v
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

type auditRecord struct {
	ID         uint64          `json:"id"`
	ActorEmail string          `json:"actor_email"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	SourceIP   string          `json:"source_ip"`
	Details    json.RawMessage `json:"details,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (a *App) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}
	ctx, cancel := contextTimeout(r, 5*time.Second)
	defer cancel()
	rows, err := a.db.QueryContext(ctx, `SELECT l.id, COALESCE(u.email, ''), l.action, l.target_type,
		l.target_id, l.source_ip, l.details, l.created_at
		FROM audit_logs l LEFT JOIN users u ON u.id = l.actor_user_id
		ORDER BY l.id DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load audit events")
		return
	}
	defer rows.Close()
	items := make([]auditRecord, 0)
	for rows.Next() {
		var item auditRecord
		var details []byte
		if err := rows.Scan(&item.ID, &item.ActorEmail, &item.Action, &item.TargetType,
			&item.TargetID, &item.SourceIP, &details, &item.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "unable to read audit events")
			return
		}
		if len(details) > 0 && json.Valid(details) {
			item.Details = append(json.RawMessage(nil), details...)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to read audit events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Commit the state change and audit event atomically; an audit failure rolls back.
func (a *App) withAudit(ctx context.Context, r *http.Request, actor int64, action, targetType, id string, details any, change func(*sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = change(tx); err != nil {
		return err
	}
	if err = auditWrite(ctx, tx, r, actor, action, targetType, id, details); err != nil {
		return err
	}
	return tx.Commit()
}
func (a *App) saveTest(ctx context.Context, r *http.Request, actor int64, id, status, version string, count int, lastError string) error {
	return a.withAudit(ctx, r, actor, "proxmox.server.test", "proxmox_server", id, map[string]string{"status": status}, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE proxmox_servers SET status = ?, version = ?, node_count = ?, last_checked_at = UTC_TIMESTAMP(), last_error = ? WHERE id = ?`, status, version, count, lastError, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var exists string
			return tx.QueryRowContext(ctx, `SELECT id FROM proxmox_servers WHERE id = ?`, id).Scan(&exists)
		}
		return nil
	})
}
