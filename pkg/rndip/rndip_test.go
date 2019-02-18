package rndip

import (
	"math/rand"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UTC().UnixNano())
	os.Exit(m.Run())
}

func badCidr(t *testing.T, cidr string) {
	if _, err := New(cidr); err == nil {
		t.Fatal("Passed invalid cidr", cidr)
	}
}
func goodCidr(t *testing.T, cidr string) *Rndip {
	r, err := New(cidr)
	if err != nil {
		t.Fatal(cidr, err)
	}
	return r
}
func TestCreate(t *testing.T) {

	goodCidr(t, "1000::/112")
	goodCidr(t, "1000::10.0.0.0/120")
	goodCidr(t, "10.0.0.0/24")
	goodCidr(t, "10.0.0.0/32")
	goodCidr(t, "0.0.0.0/32")

	badCidr(t, "10.0.0.0/33")
	badCidr(t, "1000::/129")
	badCidr(t, "123.400.3.3/24")
}

func TestNet(t *testing.T) {
	r := goodCidr(t, "10.0.0.10/24")
	t.Log(r.GetNet())
	netstr := r.GetNet().String()
	if netstr != "10.0.0.0/24" {
		t.Fatal("Invalid net", netstr)
	}
}

func TestProto(t *testing.T) {
	a := "10.0.0.0/24"
	r := goodCidr(t, a)
	if i := r.GetIP().To4(); i == nil {
		t.Fatal("Expected ipv4 for", a)
	}

	a = "1000::/96"
	r = goodCidr(t, a)
	if i := r.GetIP().To4(); i != nil {
		t.Fatal("Expected ipv6 for", a, "got", i)
	}
}

func testIPs(t *testing.T, cidr string, n int) {
	r := goodCidr(t, cidr)
	for i := 0; i < n; i++ {
		ip := r.GetIP()
		t.Log("Ip", ip)
		if !r.net.Contains(ip) {
			t.Fatal(ip, "is not in", r.net)
		}
	}
}

func TestIps(t *testing.T) {
	testIPs(t, "10.0.0.0/30", 10)
	testIPs(t, "10.0.0.0/12", 10)
	testIPs(t, "1000::/124", 10)
	testIPs(t, "1000::/16", 10)
}
