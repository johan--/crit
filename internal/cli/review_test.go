package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTmuxPaneCommand(t *testing.T) {
	cmd := buildTmuxPaneCommand("/usr/local/bin/crit", "/home/user/doc.md", "crit-review-1234")

	if !strings.Contains(cmd, "'/usr/local/bin/crit'") {
		t.Errorf("expected escaped crit binary in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "'/home/user/doc.md'") {
		t.Errorf("expected escaped file path in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "tmux wait-for -S crit-review-1234") {
		t.Errorf("expected wait-for signal in command, got: %s", cmd)
	}
}

func TestBuildTmuxPaneCommandEscapesQuotes(t *testing.T) {
	cmd := buildTmuxPaneCommand("/bin/crit", "/home/user/it's a file.md", "ch-1")

	if !strings.Contains(cmd, "'/home/user/it'\\''s a file.md'") {
		t.Errorf("expected escaped single quotes in path, got: %s", cmd)
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
	}

	for _, tt := range tests {
		got := shellEscape(tt.input)
		if got != tt.expected {
			t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetachRequiresTmux(t *testing.T) {
	tmp, err := os.CreateTemp("", "crit-test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	orig := os.Getenv("TMUX")
	os.Setenv("TMUX", "")
	defer os.Setenv("TMUX", orig)

	err = runDetachedReview(tmp.Name())
	if err == nil {
		t.Fatal("expected error when TMUX is not set")
	}
	if !strings.Contains(err.Error(), "requires a tmux session") {
		t.Errorf("expected 'requires a tmux session' error, got: %s", err)
	}
}

func TestPathResolution(t *testing.T) {
	rel := "relative/path/doc.md"
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(cwd, rel)
	if abs != expected {
		t.Errorf("filepath.Abs(%q) = %q, want %q", rel, abs, expected)
	}
}
