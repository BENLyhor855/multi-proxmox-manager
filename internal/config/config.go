package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv        string
	PublicOrigin  string
	Addr          string
	MySQLDSN      string
	JWTSecret     []byte
	EncryptionKey []byte
	CookieSecure  bool
	AllowedCIDRs  []*net.IPNet
	PVECACertFile string
}

func Load() (Config, error) {
	var c Config
	c.AppEnv = env("APP_ENV", "production")
	if c.AppEnv != "production" && c.AppEnv != "development" {
		return c, errors.New("APP_ENV must be production or development")
	}
	c.PublicOrigin = strings.TrimRight(strings.TrimSpace(os.Getenv("APP_ORIGIN")), "/")
	if c.PublicOrigin != "" || c.AppEnv == "production" {
		u, e := url.Parse(c.PublicOrigin)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("APP_ORIGIN must be the public HTTPS origin in production")
		}
	}
	c.Addr = env("HTTP_ADDR", "127.0.0.1:8080")
	if c.AppEnv == "production" {
		host, _, err := net.SplitHostPort(c.Addr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			return c, errors.New("HTTP_ADDR must bind to a loopback IP in production")
		}
	}
	c.MySQLDSN = strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	if c.MySQLDSN == "" {
		return c, errors.New("MYSQL_DSN is required")
	}

	jwt := []byte(os.Getenv("JWT_SECRET"))
	if len(jwt) < 32 || strings.Contains(string(jwt), "REPLACE_WITH") {
		return c, errors.New("JWT_SECRET must contain at least 32 bytes; generate with: openssl rand -base64 48")
	}
	c.JWTSecret = jwt

	enc, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("CREDENTIALS_ENCRYPTION_KEY")))
	if err != nil || len(enc) != 32 {
		return c, errors.New("CREDENTIALS_ENCRYPTION_KEY must be base64-encoded 32-byte key; generate with: openssl rand -base64 32")
	}
	c.EncryptionKey = enc

	c.CookieSecure, err = strconv.ParseBool(env("COOKIE_SECURE", "true"))
	if err != nil {
		return c, fmt.Errorf("COOKIE_SECURE must be true or false: %w", err)
	}
	if c.AppEnv == "production" && !c.CookieSecure {
		return c, errors.New("COOKIE_SECURE must be true in production")
	}
	c.PVECACertFile = strings.TrimSpace(os.Getenv("PVE_CA_FILE"))

	cidrs := strings.Split(strings.TrimSpace(os.Getenv("PVE_ALLOWED_CIDRS")), ",")
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, n, parseErr := net.ParseCIDR(raw)
		if parseErr != nil {
			return c, fmt.Errorf("invalid PVE_ALLOWED_CIDRS entry %q: %w", raw, parseErr)
		}
		c.AllowedCIDRs = append(c.AllowedCIDRs, n)
	}
	if len(c.AllowedCIDRs) == 0 {
		return c, errors.New("PVE_ALLOWED_CIDRS is required; configure only the networks that are allowed to host Proxmox management endpoints")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
