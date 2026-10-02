package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var Templates embed.FS

func SetTemplates(templates embed.FS) {
	Templates = templates
}

func Extract(srcDir, destDir string, force bool) error {
	return fs.WalkDir(Templates, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		relSlash := filepath.ToSlash(relPath)
		switch {
		case relSlash == "kiro" || strings.HasPrefix(relSlash, "kiro/"):
			relSlash = ".kiro" + strings.TrimPrefix(relSlash, "kiro")
		case relSlash == "kairon" || strings.HasPrefix(relSlash, "kairon/"):
			relSlash = ".kairon" + strings.TrimPrefix(relSlash, "kairon")
		}

		destPath := filepath.Join(destDir, filepath.FromSlash(relSlash))

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		if !force {
			if _, err := os.Stat(destPath); err == nil {
				return nil
			}
		}

		// config.yaml is NEVER overwritten (preserve user settings)
		configPath := filepath.ToSlash(destPath)
		if strings.HasSuffix(configPath, "/.kairon/config.yaml") || strings.HasSuffix(configPath, ".kairon/config.yaml") {
			if _, err := os.Stat(destPath); err == nil {
				return nil
			}
		}

		data, err := fs.ReadFile(Templates, path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return err
		}

		fmt.Printf("Extracted: %s\n", destPath)
		return nil
	})
}
