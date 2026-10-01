// Package fakenet runs sites on fake domains over real HTTPS, inside one
// process: one server that routes by host, a throwaway certificate authority
// that issues a certificate for each domain on demand, and a client that
// trusts it and reaches every registered domain. TLS is fully verified.
//
// Tests use it through package testnet; sourced-lab's demo uses it directly.
package fakenet

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Network is a set of fake-domain HTTPS sites.
type Network struct {
	srv *httptest.Server
	ca  *authority

	mu       sync.Mutex
	handlers map[string]http.Handler
	faults   map[string]*Faults
	reqs     map[string]*counter
}

// Start starts an empty network. Call Close when done.
func Start() (*Network, error) {
	ca, err := newAuthority()
	if err != nil {
		return nil, err
	}
	n := &Network{
		ca:       ca,
		handlers: map[string]http.Handler{},
		faults:   map[string]*Faults{},
		reqs:     map[string]*counter{},
	}
	n.srv = httptest.NewUnstartedServer(http.HandlerFunc(n.route))
	n.srv.TLS = &tls.Config{GetCertificate: ca.certificate}
	n.srv.StartTLS()
	return n, nil
}

// Close shuts the network down.
func (n *Network) Close() { n.srv.Close() }

// AddRoot serves a web root on domain the way a well-configured static host
// would, and returns the site's fault switches.
func (n *Network) AddRoot(domain, root string) *Faults {
	f, c := &Faults{}, &counter{}
	n.mu.Lock()
	defer n.mu.Unlock()
	domain = strings.ToLower(domain)
	n.faults[domain], n.reqs[domain] = f, c
	n.handlers[domain] = siteHandler(root, f, c)
	return f
}

// AddHandler serves h on domain, e.g. a resolver on resolver.test.
func (n *Network) AddHandler(domain string, h http.Handler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers[strings.ToLower(domain)] = h
}

// Faults returns a site's fault switches, or nil if domain isn't a site.
func (n *Network) Faults(domain string) *Faults {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.faults[strings.ToLower(domain)]
}

// Requests returns how many requests a site received whose method and path
// start with prefix, e.g. "GET /.well-known/sourced/bundles/".
func (n *Network) Requests(domain, prefix string) int {
	n.mu.Lock()
	c := n.reqs[strings.ToLower(domain)]
	n.mu.Unlock()
	if c == nil {
		return 0
	}
	return c.count(prefix)
}

// Client returns an HTTP client that reaches every site on the network and
// trusts only the network's certificate authority. Unknown domains fail as
// if they had no DNS record.
func (n *Network) Client() *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(n.ca.cert)
	addr := n.srv.Listener.Addr().String()
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool},
		DialContext: func(ctx context.Context, network, hostport string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(hostport)
			if err != nil {
				return nil, err
			}
			n.mu.Lock()
			_, known := n.handlers[strings.ToLower(host)]
			n.mu.Unlock()
			if !known {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2: true,
	}}
}

// CACertPEM returns the network's root certificate in PEM form.
func (n *Network) CACertPEM() []byte { return n.ca.pem }

func (n *Network) route(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	n.mu.Lock()
	h := n.handlers[host]
	n.mu.Unlock()
	if h == nil {
		http.Error(w, "unknown host", http.StatusMisdirectedRequest)
		return
	}
	h.ServeHTTP(w, r)
}

// authority is a throwaway certificate authority that issues a leaf
// certificate for each server name on first use.
type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte

	mu     sync.Mutex
	leaves map[string]*tls.Certificate
}

func newAuthority() (*authority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sourced.net testnet CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return &authority{cert: cert, key: key, pem: certPEM, leaves: map[string]*tls.Certificate{}}, nil
}

func (a *authority) certificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(hello.ServerName)
	if name == "" {
		return nil, fmt.Errorf("no server name in TLS handshake")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.leaves[name]; ok {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		return nil, err
	}
	c := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	a.leaves[name] = c
	return c, nil
}

// CopyTree copies the files under src into dst.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
