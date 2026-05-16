package cmd

import (
	"github.com/spf13/cobra"

	"github.com/kdubb1337/ghub/internal/api"
	"github.com/kdubb1337/ghub/internal/output"
)

// `ghub branch` — branch read commands.

var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Inspect branches",
}

var (
	branchListRepo      string
	branchListProtected bool
	branchListAll       bool
	branchListLimit     int
	branchListCursor    string
)

var branchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List branches for a repository",
	Example: `  # Branches of the repo in cwd
  ghub branch list --json

  # Explicit repo, protected branches only
  ghub branch list --repo acme/widget --protected`,
	RunE: func(cmd *cobra.Command, args []string) error {
		owner, repo, err := resolveRepo(branchListRepo)
		if err != nil {
			return err
		}
		c, err := newAPIClient()
		if err != nil {
			return err
		}
		ctx, cancel := cmdContext()
		defer cancel()
		opts := api.ListBranchesOpts{
			Owner: owner, Repo: repo,
			Limit:  branchListLimit,
			Cursor: branchListCursor,
		}
		// --protected filters in; --all overrides to nil (the default).
		if cmd.Flags().Changed("protected") && !branchListAll {
			p := branchListProtected
			opts.Protected = &p
		}
		branches, next, err := c.ListBranches(ctx, opts)
		if err != nil {
			return err
		}
		if next != "" {
			return output.EmitPage(branches, next, "more available; pass --cursor=<value>")
		}
		return output.Emit(branches)
	},
}

func init() {
	branchListCmd.Flags().StringVar(&branchListRepo, "repo", "", "owner/name (default: detect from cwd git remote)")
	branchListCmd.Flags().BoolVar(&branchListProtected, "protected", false, "only protected branches")
	branchListCmd.Flags().BoolVar(&branchListAll, "all", false, "ignore --protected and return all branches")
	branchListCmd.Flags().IntVar(&branchListLimit, "limit", 25, "max items to return")
	branchListCmd.Flags().StringVar(&branchListCursor, "cursor", "", "pagination cursor from previous response")

	branchCmd.AddCommand(branchListCmd)
	rootCmd.AddCommand(branchCmd)
}
