// Package sites holds the sample sites used by tests and the demo: a news
// site, a library's guides, software docs, a long essay, and a site that
// doesn't take part. Each is a folder holding public/; a PLAIN file marks a
// site that publishes no signed content.
package sites

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed *.test
var files embed.FS

// FS returns the sample sites, one folder per domain.
func FS() fs.FS { return files }

// Write copies the sample sites into dir, one folder per domain.
func Write(dir string) error {
	return fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
