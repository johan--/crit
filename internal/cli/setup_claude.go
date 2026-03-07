package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed skill/crit-review/SKILL.md
var skillContent embed.FS

var setupProject bool
var setupForce bool

var setupClaudeCmd = &cobra.Command{
	Use:   "setup-claude",
	Short: "Install Claude Code skill for crit review workflow",
	Long:  "Installs the /crit-review skill to ~/.claude/skills/ (or .claude/skills/ with --project). Alternative to installing the crit plugin via /plugin install.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetDir string

		if setupProject {
			targetDir = filepath.Join(".claude", "skills", "crit-review")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not determine home directory: %w", err)
			}
			targetDir = filepath.Join(home, ".claude", "skills", "crit-review")
		}

		targetPath := filepath.Join(targetDir, "SKILL.md")

		if !setupForce {
			if _, err := os.Stat(targetPath); err == nil {
				return fmt.Errorf("skill already exists at %s (use --force to overwrite)", targetPath)
			}
		}

		content, err := skillContent.ReadFile("skill/crit-review/SKILL.md")
		if err != nil {
			return fmt.Errorf("reading embedded skill: %w", err)
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", targetDir, err)
		}

		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("writing skill file: %w", err)
		}

		scope := "globally"
		if setupProject {
			scope = "for this project"
		}
		fmt.Printf("Installed Claude Code skill %s to %s\n", scope, targetPath)
		fmt.Println("You can now use /crit-review <path> in Claude Code.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupClaudeCmd)
	setupClaudeCmd.Flags().BoolVar(&setupProject, "project", false, "install to .claude/skills/ in the current directory instead of globally")
	setupClaudeCmd.Flags().BoolVar(&setupForce, "force", false, "overwrite existing skill file")
}
