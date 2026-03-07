package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/review"
)

var statusCmd = &cobra.Command{
	Use:   "status <file>",
	Short: "Show review status as JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		state, err := review.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(state); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
