package document

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

const (
	critDir    = ".crit"
	reviewsDir = "reviews"
)

func EnsureDirs() error {
	dir := filepath.Join(critDir, reviewsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	gitignorePath := filepath.Join(critDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte("*\n"), 0644); err != nil {
			return fmt.Errorf("creating .gitignore: %w", err)
		}
	}

	return nil
}

func ReviewPath(docPath string) string {
	abs, err := filepath.Abs(docPath)
	if err != nil {
		abs = docPath
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(abs)))
	return filepath.Join(critDir, reviewsDir, hash+".yaml")
}
