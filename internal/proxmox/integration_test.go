package proxmox

import (
	"context"
	"example.com/multi-proxmox-manager/internal/testutil"
	"net"
	"testing"
)

func TestIndependentTLSInstallations(t *testing.T) {
	one, ca1 := testutil.PVE(t, "127.0.0.2", "secret-a", "8.4")
	two, ca2 := testutil.PVE(t, "127.0.0.3", "secret-b", "9.0")
	_, cidr, _ := net.ParseCIDR("127.0.0.0/8")
	client, err := NewClient([]*net.IPNet{cidr}, testutil.CAFile(t, ca1, ca2))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []testutil.Fixture{one, two} {
		got, err := client.Test(context.Background(), f.URL, f.TokenID, f.Secret)
		if err != nil || got.Version != f.Version || got.NodeCount != 1 {
			t.Fatalf("%s: %#v %v", f.URL, got, err)
		}
	}
	if _, err := client.Test(context.Background(), two.URL, two.TokenID, one.Secret); err == nil {
		t.Fatal("cross-installation credential accepted")
	}
	untrusted, _ := NewClient([]*net.IPNet{cidr}, "")
	if _, err := untrusted.Test(context.Background(), one.URL, one.TokenID, one.Secret); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	_, blocked, _ := net.ParseCIDR("192.0.2.0/24")
	denied, _ := NewClient([]*net.IPNet{blocked}, testutil.CAFile(t, ca1))
	if _, err := denied.Test(context.Background(), one.URL, one.TokenID, one.Secret); err == nil {
		t.Fatal("CIDR bypass")
	}
}
