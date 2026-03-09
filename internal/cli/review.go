package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/git"
	"github.com/kevindutra/crit/internal/review"
	"github.com/kevindutra/crit/internal/tui"
)

var reviewDetach bool
var reviewWait bool
var reviewCode bool

var reviewCmd = &cobra.Command{
	Use:   "review [file]",
	Short: "Launch interactive TUI review",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if reviewCode {
			return runCodeReview()
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for code review mode)")
		}

		filePath := args[0]

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", filePath)
		}

		// --wait without --detach: warn and ignore
		if reviewWait && !reviewDetach {
			fmt.Fprintln(os.Stderr, "crit: --wait requires --detach; ignoring --wait")
			reviewWait = false
		}

		if reviewDetach {
			return runDetachedReview(filePath)
		}

		model := tui.NewApp(filePath)
		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().BoolVar(&reviewDetach, "detach", false, "open review in a tmux split pane")
	reviewCmd.Flags().BoolVar(&reviewWait, "wait", false, "block until the detached review completes (requires --detach)")
	reviewCmd.Flags().BoolVar(&reviewCode, "code", false, "review code changes (multi-file mode)")
}

func runCodeReview() error {
	// --wait without --detach: warn and ignore
	if reviewWait && !reviewDetach {
		fmt.Fprintln(os.Stderr, "crit: --wait requires --detach; ignoring --wait")
		reviewWait = false
	}

	if reviewDetach {
		return runDetachedCodeReview()
	}

	if !git.IsGitRepo() {
		return fmt.Errorf("crit review --code requires a git repository")
	}

	files, err := git.ChangedFiles()
	if err != nil {
		return fmt.Errorf("detecting changed files: %w", err)
	}

	ref := "HEAD"

	if len(files) == 0 {
		// Interactive fallback: try other refs
		var fallbackErr error
		ref, files, fallbackErr = interactiveFallback()
		if fallbackErr != nil {
			return fallbackErr
		}
	}

	// Save session manifest
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	session := &review.CodeReviewSession{
		Files:     paths,
		DiffBase:  ref,
		CreatedAt: time.Now(),
	}
	if err := review.SaveSession(session); err != nil {
		fmt.Fprintf(os.Stderr, "crit: warning: could not save session: %v\n", err)
	}

	model := tui.NewCodeReviewApp(files, ref)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func runDetachedCodeReview() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("--detach requires a tmux session (TMUX environment variable not set)")
	}

	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux binary not found on PATH: %w", err)
	}

	critBin, err := resolveExecutable()
	if err != nil {
		return fmt.Errorf("resolving crit binary path: %w", err)
	}

	channel := fmt.Sprintf("crit-review-%d", os.Getpid())
	critCmd := fmt.Sprintf("CRIT_DETACHED=1 %s review --code ; tmux wait-for -S %s",
		shellEscape(critBin), channel)

	splitCmd := exec.Command(tmuxBin, "split-window", "-h", "-p", "70", critCmd)
	if err := splitCmd.Run(); err != nil {
		return fmt.Errorf("failed to open tmux pane: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Opened code review in tmux pane")

	if reviewWait {
		waitCmd := exec.Command(tmuxBin, "wait-for", channel)
		if err := waitCmd.Run(); err != nil {
			return fmt.Errorf("review pane terminated abnormally")
		}

		fmt.Fprintln(os.Stdout, "Code review complete.")
	}

	return nil
}

func interactiveFallback() (string, []git.FileChange, error) {
	// Try common alternatives in order
	alternatives := []struct {
		label string
		ref   string
	}{
		{"last commit (HEAD~1)", "HEAD~1"},
		{"base branch (main)", "main"},
	}

	for _, alt := range alternatives {
		files, err := git.ChangedFilesFrom(alt.ref)
		if err != nil {
			continue
		}
		if len(files) > 0 {
			fmt.Fprintf(os.Stderr, "No unstaged changes found. Using %s.\n", alt.label)
			return alt.ref, files, nil
		}
	}

	return "", nil, fmt.Errorf("no changed files found")
}

func runDetachedReview(filePath string) error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("--detach requires a tmux session (TMUX environment variable not set)")
	}

	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux binary not found on PATH: %w", err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	critBin, err := resolveExecutable()
	if err != nil {
		return fmt.Errorf("resolving crit binary path: %w", err)
	}

	channel := fmt.Sprintf("crit-review-%d", os.Getpid())
	critCmd := buildTmuxPaneCommand(critBin, absPath, channel)

	splitCmd := exec.Command(tmuxBin, "split-window", "-h", "-p", "70", critCmd)
	if err := splitCmd.Run(); err != nil {
		return fmt.Errorf("failed to open tmux pane: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Opened review in tmux pane")

	if reviewWait {
		waitCmd := exec.Command(tmuxBin, "wait-for", channel)
		if err := waitCmd.Run(); err != nil {
			return fmt.Errorf("review pane terminated abnormally")
		}

		state, err := review.Load(absPath)
		if err != nil {
			return fmt.Errorf("reading review state: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Review complete. %d comments.\n", len(state.Comments))
	}

	return nil
}

// resolveExecutable returns the absolute path to the currently running binary.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// buildTmuxPaneCommand constructs the shell command string to run inside a tmux split pane.
// It runs crit review for the given file, then signals the wait-for channel on completion.
func buildTmuxPaneCommand(critBin, absPath, channel string) string {
	return fmt.Sprintf("CRIT_DETACHED=1 %s review %s ; tmux wait-for -S %s",
		shellEscape(critBin), shellEscape(absPath), channel)
}

// shellEscape escapes a string for safe embedding in a POSIX shell command.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
