// Package helper embeds the Python safetensors helper so a single lvwb binary can run it.
package helper

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
)

//go:embed hf_serve.py pyproject.toml
var files embed.FS

// Unpack writes the helper into dir, leaving unchanged files alone so uv keeps its venv.
func Unpack(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"hf_serve.py", "pyproject.toml"} {
		b, _ := files.ReadFile(name)
		dst := filepath.Join(dir, name)
		if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, b) {
			continue
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
