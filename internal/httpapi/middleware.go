package httpapi

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Reject cross-origin browser mutations, including login CSRF. Non-browser clients
// may omit Origin, but JSON mutations cannot be submitted by a simple HTML form.
func sameOrigin(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeError(w, 403, "cross-origin request denied")
				return
			}
			if raw := r.Header.Get("Origin"); raw != "" {
				u, err := url.Parse(raw)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" || (origin != "" && raw != origin) {
					writeError(w, 403, "cross-origin request denied")
					return
				}
			}
			if r.Method == "POST" {
				media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || media != "application/json" {
					writeError(w, 415, "application/json required")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Bounded memory and Argon2 concurrency, including when run without Nginx.
func loginThrottle(next http.Handler) http.Handler {
	type entry struct {
		since time.Time
		count int
	}
	var mu sync.Mutex
	entries := map[string]entry{}
	slots := make(chan struct{}, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		ip := sourceIP(r)
		mu.Lock()
		for k, v := range entries {
			if now.Sub(v.since) >= time.Minute {
				delete(entries, k)
			}
		}
		v := entries[ip]
		denied := v.count >= 10 || (v.count == 0 && len(entries) >= 10000)
		if !denied {
			if v.count == 0 {
				v.since = now
			}
			v.count++
			entries[ip] = v
		}
		mu.Unlock()
		if denied {
			w.Header().Set("Retry-After", "60")
			writeError(w, 429, "too many attempts")
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			writeError(w, 429, "authentication busy")
		}
	})
}
