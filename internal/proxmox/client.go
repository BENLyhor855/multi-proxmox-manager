package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	allowed []*net.IPNet
	client  *http.Client
}

type Node struct {
	Node   string   `json:"node"`
	Status string   `json:"status"`
	CPU    *float64 `json:"cpu,omitempty"`
	MaxCPU *int64   `json:"maxcpu,omitempty"`
	Mem    *int64   `json:"mem,omitempty"`
	MaxMem *int64   `json:"maxmem,omitempty"`
}

type TestResult struct {
	Version   string `json:"version"`
	Release   string `json:"release"`
	NodeCount int    `json:"node_count"`
	Nodes     []Node `json:"nodes"`
}

type envelope[T any] struct {
	Data T `json:"data"`
}

func NewClient(allowed []*net.IPNet, caFile string) (*Client, error) {
	if len(allowed) == 0 {
		return nil, errors.New("no Proxmox destination networks configured")
	}
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if caFile != "" {
		pem, readErr := os.ReadFile(caFile)
		if readErr != nil {
			return nil, fmt.Errorf("read PVE_CA_FILE: %w", readErr)
		}
		if ok := rootCAs.AppendCertsFromPEM(pem); !ok {
			return nil, errors.New("PVE_CA_FILE contains no valid CA certificates")
		}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil, // Do not allow proxy environment variables to bypass destination checks.
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootCAs},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   4,
		MaxConnsPerHost:       4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			p, parseErr := strconv.Atoi(port)
			if parseErr != nil || p != 8006 {
				return nil, errors.New("Proxmox API connection must use TCP port 8006")
			}
			ips, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
			if lookupErr != nil {
				return nil, lookupErr
			}
			var lastErr error
			for _, candidate := range ips {
				if !containsIP(allowed, candidate.IP) {
					lastErr = fmt.Errorf("resolved Proxmox address %s is outside PVE_ALLOWED_CIDRS", candidate.IP)
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(candidate.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr == nil {
				lastErr = errors.New("no allowed Proxmox address found")
			}
			return nil, lastErr
		},
	}
	return &Client{
		allowed: allowed,
		client: &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func containsIP(cidrs []*net.IPNet, ip net.IP) bool {
	for _, n := range cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func ValidateBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("invalid Proxmox URL")
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || strings.Contains(u.Host, "%") {
		return "", errors.New("Proxmox URL must be an HTTPS origin without credentials, query or fragment")
	}
	if u.Port() != "8006" {
		return "", errors.New("Proxmox URL must use port 8006")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("Proxmox URL must not include a path")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = ""
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) Test(ctx context.Context, baseURL, tokenID, tokenSecret string) (TestResult, error) {
	var result TestResult
	base, err := ValidateBaseURL(baseURL)
	if err != nil {
		return result, err
	}
	if tokenID == "" || tokenSecret == "" || strings.ContainsAny(tokenID, "\r\n") || strings.ContainsAny(tokenSecret, "\r\n") {
		return result, errors.New("invalid Proxmox API token credentials")
	}

	var version struct {
		Version string `json:"version"`
		Release string `json:"release"`
	}
	if err := c.get(ctx, base+"/api2/json/version", tokenID, tokenSecret, &version); err != nil {
		return result, err
	}
	var nodes []Node
	if err := c.get(ctx, base+"/api2/json/nodes", tokenID, tokenSecret, &nodes); err != nil {
		return result, err
	}
	if version.Version == "" || len(version.Version) > 80 || len(version.Release) > 80 || len(nodes) == 0 {
		return result, errors.New("Proxmox response has no valid version or visible nodes; check ACL permissions")
	}
	result.Version = version.Version
	result.Release = version.Release
	result.NodeCount = len(nodes)
	result.Nodes = nodes
	return result, nil
}

func (c *Client) get(ctx context.Context, endpoint, tokenID, tokenSecret string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("unable to prepare Proxmox API request")
	}
	req.Header.Set("Authorization", "PVEAPIToken="+tokenID+"="+tokenSecret)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("Proxmox API connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Proxmox API returned HTTP %d; check the token ID, token secret and ACL permissions", resp.StatusCode)
	}
	var body envelope[json.RawMessage]
	payload, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(payload) > 2<<20 {
		return errors.New("Proxmox API response exceeds size limit or could not be read")
	}
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	if err := dec.Decode(&body); err != nil {
		return errors.New("Proxmox API returned an invalid JSON response")
	}
	if len(body.Data) == 0 || string(body.Data) == "null" {
		return errors.New("Proxmox API response has no data")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("Proxmox API returned trailing response data")
	}
	if err := json.Unmarshal(body.Data, target); err != nil {
		return errors.New("Proxmox API response did not match the expected format")
	}
	return nil
}
