// Package testnet runs the sample publishers inside a test, on a fakenet
// network: real HTTPS on fake domains, with fault switches for chaos
// testing (tampered bundles, wrong or revoked keys, rolled-back manifests,
// missing links, slow, failing, or dead servers).
package testnet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcednet/testkit/fakenet"
	"github.com/sourcednet/testkit/sites"
	"github.com/sourcednet/testkit/testsite"
)

// Faults are a site's chaos switches.
type Faults = fakenet.Faults

// Network is a fakenet network tied to a test.
type Network struct {
	*fakenet.Network
	t     testing.TB
	sites map[string]*testsite.Site
}

// New starts an empty network. It shuts down when the test ends.
func New(t testing.TB) *Network {
	t.Helper()
	fn, err := fakenet.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fn.Close)
	return &Network{Network: fn, t: t, sites: map[string]*testsite.Site{}}
}

// Standard returns a network with every sample site (package sites),
// each built once at testsite.T0.
func Standard(t testing.TB) *Network {
	t.Helper()
	n := New(t)
	dir := SitesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		src := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(src, "PLAIN")); err == nil {
			n.AddPlainSite(e.Name(), filepath.Join(src, "public"))
			continue
		}
		s := n.AddSite(e.Name())
		if err := fakenet.CopyTree(filepath.Join(src, "public"), s.Root()); err != nil {
			t.Fatal(err)
		}
		s.Build(testsite.T0, nil)
	}
	return n
}

// SitesDir writes the sample sites (package sites) to a temporary directory
// and returns it.
func SitesDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if err := sites.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// AddSite creates an empty publisher project for domain and serves its web root.
func (n *Network) AddSite(domain string) *testsite.Site {
	n.t.Helper()
	s := testsite.New(n.t, domain)
	n.sites[domain] = s
	n.AddRoot(domain, s.Root())
	return s
}

// AddPlainSite serves dir as an ordinary website that doesn't take part in sourced.net.
func (n *Network) AddPlainSite(domain, dir string) { n.AddRoot(domain, dir) }

// Site returns a publisher site by domain.
func (n *Network) Site(domain string) *testsite.Site {
	n.t.Helper()
	s, ok := n.sites[domain]
	if !ok {
		n.t.Fatalf("no publisher site %s", domain)
	}
	return s
}

// Faults returns the fault switches of a site.
func (n *Network) Faults(domain string) *Faults {
	n.t.Helper()
	f := n.Network.Faults(domain)
	if f == nil {
		n.t.Fatalf("no site %s", domain)
	}
	return f
}
