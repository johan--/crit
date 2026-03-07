package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/review"
	"github.com/kevindutra/crit/internal/tui"
)

var reviewDetach bool
var reviewWait bool

var reviewCmd = &cobra.Command{
	Use:   "review <file>",
	Short: "Launch interactive TUI review",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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
