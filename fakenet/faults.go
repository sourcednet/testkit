package fakenet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sourcednet/core"
	"github.com/sourcednet/core/htmllink"
)

// Faults are a site's chaos switches. The zero value serves the site as it is.
type Faults struct {
	mu sync.Mutex
	s  faultState
}

type faultState struct {
	tamperBundles bool
	wrongKeys     bool
	revokeKeys    bool
	manifest      []byte
	stripLinks    bool
	noLinkHeader  bool
	delay         time.Duration
	status        int
	down          bool
}

func (f *Faults) set(fn func(*faultState)) { f.mu.Lock(); defer f.mu.Unlock(); fn(&f.s) }

// TamperBundles alters one character of every chunk text the site serves.
func (f *Faults) TamperBundles() { f.set(func(s *faultState) { s.tamperBundles = true }) }

// WrongKeys serves keys.json with a different public key under every key ID,
// as an attacker who can edit keys.json but lacks the private keys would.
func (f *Faults) WrongKeys() { f.set(func(s *faultState) { s.wrongKeys = true }) }

// RevokeKeys serves keys.json with every key marked revoked.
func (f *Faults) RevokeKeys() { f.set(func(s *faultState) { s.revokeKeys = true }) }

// ServeManifest serves these bytes instead of the site's manifest, e.g. an
// older one to simulate a rollback. Nil restores the real manifest.
func (f *Faults) ServeManifest(b []byte) { f.set(func(s *faultState) { s.manifest = b }) }

// StripLinks removes the sourced-record link from pages and their headers.
func (f *Faults) StripLinks() { f.set(func(s *faultState) { s.stripLinks = true }) }

// NoLinkHeader stops sending the Link header but keeps the <link> element.
func (f *Faults) NoLinkHeader() { f.set(func(s *faultState) { s.noLinkHeader = true }) }

// Delay holds every response for d.
func (f *Faults) Delay(d time.Duration) { f.set(func(s *faultState) { s.delay = d }) }

// Status answers every request with this HTTP status.
func (f *Faults) Status(code int) { f.set(func(s *faultState) { s.status = code }) }

// Down closes every connection without answering.
func (f *Faults) Down() { f.set(func(s *faultState) { s.down = true }) }

// Reset turns every fault off.
func (f *Faults) Reset() { f.set(func(s *faultState) { *s = faultState{} }) }

func (f *Faults) snapshot() faultState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.s
}

var linkElement = regexp.MustCompile(`[ \t]*<link rel="sourced-record" href="[^"]*">\n?`)

// counter counts requests per path.
type counter struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *counter) add(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[p]++
}

// count returns how many requests had a path starting with prefix.
func (c *counter) count(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for p, n := range c.n {
		if strings.HasPrefix(p, prefix) {
			total += n
		}
	}
	return total
}

// siteHandler serves a web root the way a well-configured static host would,
// including ETags and the recommended Link header on pages, with faults
// applied. It counts requests in reqs.
func siteHandler(root string, faults *Faults, reqs *counter) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.add(r.Method + " " + path.Clean("/"+r.URL.Path))
		f := faults.snapshot()
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		if f.down {
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
					return
				}
			}
			panic(http.ErrAbortHandler) // HTTP/2: abort the stream
		}
		if f.status != 0 {
			http.Error(w, http.StatusText(f.status), f.status)
			return
		}

		p := path.Clean("/" + r.URL.Path)
		file := filepath.Join(root, filepath.FromSlash(p))
		wk := core.WellKnownPath
		switch {
		case p == wk+"keys.json" && (f.wrongKeys || f.revokeKeys):
			serveTransformed(w, r, file, func(b []byte) []byte { return alterKeys(b, f.wrongKeys, f.revokeKeys) })
		case p == wk+"manifest.json" && f.manifest != nil:
			serveBytes(w, r, f.manifest, "application/json")
		case strings.HasPrefix(p, wk+"bundles/") && f.tamperBundles:
			serveTransformed(w, r, file, tamperBundle)
		case strings.HasPrefix(p, wk):
			if b, err := os.ReadFile(file); err == nil {
				sum := sha256.Sum256(b)
				w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:8])+`"`)
			}
			files.ServeHTTP(w, r)
		default:
			page := pageFile(root, p)
			if page == "" {
				files.ServeHTTP(w, r)
				return
			}
			b, err := os.ReadFile(page)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if f.stripLinks {
				b = linkElement.ReplaceAll(b, nil)
			}
			if href := htmllink.FromHTML(b, "https://"+r.Host+p); href != "" && !f.noLinkHeader && !f.stripLinks {
				if u := strings.TrimPrefix(href, "https://"+r.Host); u != href {
					w.Header().Set("Link", "<"+u+`>; rel="`+htmllink.Rel+`"`)
				}
			}
			serveBytes(w, r, b, "text/html; charset=utf-8")
		}
	})
}

// pageFile maps a request path to an HTML file in root, or "" if none.
func pageFile(root, p string) string {
	candidates := []string{p}
	switch {
	case strings.HasSuffix(p, "/") || p == "/":
		candidates = []string{path.Join(p, "index.html")}
	case path.Ext(p) == "":
		candidates = []string{p + ".html", path.Join(p, "index.html")}
	}
	for _, c := range candidates {
		if ext := path.Ext(c); ext != ".html" && ext != ".htm" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(c))
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
			return full
		}
	}
	return ""
}

func serveBytes(w http.ResponseWriter, r *http.Request, b []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
}

func serveTransformed(w http.ResponseWriter, r *http.Request, file string, transform func([]byte) []byte) {
	b, err := os.ReadFile(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveBytes(w, r, transform(b), "application/json")
}

func alterKeys(b []byte, wrong, revoke bool) []byte {
	var ks core.KeySet
	if json.Unmarshal(b, &ks) != nil {
		return b
	}
	for i := range ks.Keys {
		if wrong {
			seed := sha256.Sum256([]byte("attacker " + ks.Keys[i].ID))
			pub := ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey)
			ks.Keys[i].PublicKey = base64.StdEncoding.EncodeToString(pub)
		}
		if revoke {
			ks.Keys[i].Status = core.KeyRevoked
		}
	}
	out, _ := core.EncodeJSON(ks)
	return out
}

func tamperBundle(b []byte) []byte {
	var bundle core.Bundle
	if json.Unmarshal(b, &bundle) != nil {
		return b
	}
	for i := range bundle.Chunks {
		t := []rune(bundle.Chunks[i].Text)
		if len(t) > 0 {
			t[len(t)/2] = '#'
			bundle.Chunks[i].Text = string(t)
		}
	}
	out, _ := core.EncodeJSON(bundle)
	return out
}
