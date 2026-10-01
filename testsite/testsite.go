// Package testsite creates publisher projects with realistic pages for
// tests: a sourced.json, keys, and a web root of HTML and Markdown pages.
package testsite

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sourcednet/core"
	"github.com/sourcednet/publisher"
)

// T0 is the default time of the first key and build.
var T0 = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

// Site is a publisher project in a temporary directory.
type Site struct {
	t      testing.TB
	Dir    string
	Config *publisher.Config
}

// New creates a project for domain, initialized at T0.
func New(t testing.TB, domain string) *Site {
	t.Helper()
	dir := t.TempDir()
	c, err := publisher.Init(dir, domain, "public", T0)
	if err != nil {
		t.Fatal(err)
	}
	return &Site{t: t, Dir: dir, Config: c}
}

// Root is the web root.
func (s *Site) Root() string { return s.Config.RootDir() }

// URL is the public URL of a path, e.g. URL("/guides/a.html").
func (s *Site) URL(path string) string { return "https://" + s.Config.Publisher + path }

// WritePage writes an HTML page with site chrome around a <main> holding the
// title as <h1> and each section as <h2> plus paragraphs. Sections alternate
// heading, paragraph, paragraph, …; a section starts at every "## " string.
func (s *Site) WritePage(rel, title string, body ...string) {
	s.t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>%s</title>\n</head>\n<body>\n", html.EscapeString(title))
	b.WriteString("<header><a href=\"/\">Site header</a></header>\n<nav><a href=\"/guides/\">Site navigation</a></nav>\n<main>\n")
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(title))
	for _, part := range body {
		if h, ok := strings.CutPrefix(part, "## "); ok {
			fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(h))
		} else {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(part))
		}
	}
	b.WriteString("</main>\n<footer>Site footer</footer>\n</body>\n</html>\n")
	s.write(rel, b.String())
}

// WriteMarkdown writes a Markdown page.
func (s *Site) WriteMarkdown(rel, md string) { s.t.Helper(); s.write(rel, md) }

// WriteFile writes any file into the web root, e.g. an edited page.
func (s *Site) WriteFile(rel, content string) { s.t.Helper(); s.write(rel, content) }

func (s *Site) write(rel, content string) {
	s.t.Helper()
	p := filepath.Join(s.Root(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// Read returns a file in the web root.
func (s *Site) Read(rel string) string {
	s.t.Helper()
	b, err := os.ReadFile(filepath.Join(s.Root(), filepath.FromSlash(rel)))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(b)
}

// Remove deletes a file from the web root.
func (s *Site) Remove(rel string) {
	s.t.Helper()
	if err := os.Remove(filepath.Join(s.Root(), filepath.FromSlash(rel))); err != nil {
		s.t.Fatal(err)
	}
}

// Build runs a build and fails the test on error.
func (s *Site) Build(now time.Time, declare map[string]publisher.Declaration) *publisher.Report {
	s.t.Helper()
	rep, err := publisher.Build(s.Config, publisher.BuildOptions{Now: now, Declare: declare})
	if err != nil {
		s.t.Fatal(err)
	}
	return rep
}

// KeySet reads the site's keys.json.
func (s *Site) KeySet() *core.KeySet {
	s.t.Helper()
	ks, err := s.Config.Tree().ReadKeySet()
	if err != nil {
		s.t.Fatal(err)
	}
	return ks
}

// Record reads and verifies a record.
func (s *Site) Record(id string) *core.Record {
	s.t.Helper()
	r, err := s.Config.Tree().ReadRecord(s.KeySet(), id)
	if err != nil {
		s.t.Fatal(err)
	}
	return r
}

// Current returns the current record of a page URL per the manifest.
func (s *Site) Current(url string) *core.Record {
	s.t.Helper()
	m, err := s.Config.Tree().ReadManifest(s.KeySet())
	if err != nil || m == nil {
		s.t.Fatalf("manifest: %v", err)
	}
	id, ok := m.Entries[url]
	if !ok {
		s.t.Fatalf("%s is not in the manifest", url)
	}
	return s.Record(id)
}

// Para returns a deterministic paragraph of at least n characters about topic.
func Para(topic string, n int) string {
	var b strings.Builder
	for i := 1; b.Len() < n; i++ {
		fmt.Fprintf(&b, "The %s guide explains step %d in plain words. ", topic, i)
	}
	return strings.TrimSpace(b.String())
}

// RecordPath is the file path of a record in the web root.
func (s *Site) RecordPath(id string) string {
	s.t.Helper()
	p, err := s.Config.Tree().RecordPath(id)
	if err != nil {
		s.t.Fatal(err)
	}
	return p
}

// BundlePath is the file path of a record's bundle in the web root.
func (s *Site) BundlePath(id string) string {
	s.t.Helper()
	p, err := s.Config.Tree().BundlePath(id)
	if err != nil {
		s.t.Fatal(err)
	}
	return p
}
