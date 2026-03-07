package tui

import (
	"testing"
)

func TestNewApp(t *testing.T) {
	app := NewApp("test.md")
	if app.filePath != "test.md" {
		t.Errorf("expected filePath 'test.md', got %s", app.filePath)
	}
}
