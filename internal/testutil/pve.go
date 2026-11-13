// Package testutil provides independent TLS Proxmox API fixtures, never real PVE installations.
package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type Fixture struct {
	URL, TokenID, Secret, Version string
	Server                        *httptest.Server
}

func PVE(t *testing.T, ip, secret, version string) (Fixture, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "PVE fixture"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP(ip)}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f := Fixture{URL: "https://" + net.JoinHostPort(ip, "8006"), TokenID: "mmp@pve!test", Secret: secret, Version: version}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken="+f.TokenID+"="+f.Secret {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/json/version":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"version": version, "release": "fixture"}})
		case "/api2/json/nodes":
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"node": "pve-" + version, "status": "online"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	srv.Listener.Close()
	srv.Listener, err = net.Listen("tcp", net.JoinHostPort(ip, "8006"))
	if err != nil {
		t.Fatal(err)
	}
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	f.Server = srv
	return f, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
func CAFile(t *testing.T, certs ...[]byte) string {
	t.Helper()
	var all []byte
	for _, c := range certs {
		all = append(all, c...)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, all, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
