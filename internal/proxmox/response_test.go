package proxmox

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResponseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unauthorized", "sensitive-upstream-body", 401},
		{"forbidden", "sensitive-upstream-body", 403},
		{"redirect", "", 302},
		{"malformed", "broken", 200},
		{"null", `{"data":null}`, 200},
		{"missing", `{}`, 200},
		{"trailing", `{"data":[]} {}`, 200},
		{"oversized", strings.Repeat(" ", (2<<20)+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "PVEAPIToken=mmp@pve!test=secret" {
					t.Fatal("wrong token header")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			var nodes []Node
			err := c.get(context.Background(), "https://pve.test:8006/api2/json/nodes", "mmp@pve!test", "secret", &nodes)
			if err == nil {
				t.Fatal("bad response accepted")
			}
			if strings.Contains(err.Error(), "sensitive-upstream-body") {
				t.Fatal("upstream body leaked")
			}
		})
	}
}
