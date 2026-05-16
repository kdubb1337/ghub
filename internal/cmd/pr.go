package cmd

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/kdubb1337/ghub/internal/api"
	"github.com/kdubb1337/ghub/internal/output"
)

// `ghub pr` — pull-request commands (list, get, create).

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Manage pull requests",
}

// --- pr list ----------------------------------------------------------------

var (
	prListRepo   string
	prListState  string
	prListLimit  int
	prListCursor string
)

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests for a repo",
	Example: `  # PRs for the repo of the current working directory
  ghub pr list --json

  # Explicit repo, closed PRs
  ghub pr list --repo acme/widget --state closed --limit 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch prListState {
		case "", "open", "closed", "all":
		default:
			return output.ErrorfEnum(9, "validation",
				[]string{"open", "closed", "all"},
				"invalid --state %q", prListState)
		}
		owner, repo, err := resolveRepo(prListRepo)
		if err != nil {
			return err
		}
		c, err := newAPIClient()
		if err != nil {
			return err
		}
		ctx, cancel := cmdContext()
		defer cancel()
		prs, next, err := c.ListPRs(ctx, api.ListPRsOpts{
			Owner: owner, Repo: repo,
			State:  prListState,
			Limit:  prListLimit,
			Cursor: prListCursor,
		})
		if err != nil {
			return err
		}
		if next != "" {
			return output.EmitPage(prs, next, "more available; pass --cursor=<value>")
		}
		return output.Emit(prs)
	},
}

// --- pr get -----------------------------------------------------------------

var prGetRepo string

var prGetCmd = &cobra.Command{
	Use:   "get <number>",
	Short: "Get a single pull request",
	Args:  cobra.ExactArgs(1),
	Example: `  ghub pr get 42 --json
  ghub pr get 42 --repo acme/widget --compact`,
	RunE: func(cmd *cobra.Command, args []string) error {
		num, err := strconv.Atoi(args[0])
		if err != nil || num <= 0 {
			return output.Errorf(2, "usage", "expected a positive PR number, got %q", args[0])
		}
		owner, repo, err := resolveRepo(prGetRepo)
		if err != nil {
			return err
		}
		c, err := newAPIClient()
		if err != nil {
			return err
		}
		ctx, cancel := cmdContext()
		defer cancel()
		pr, err := c.GetPR(ctx, owner, repo, num)
		if err != nil {
			return err
		}
		return output.Emit(pr)
	},
}

// --- pr create --------------------------------------------------------------

var (
	prCreateRepo  string
	prCreateTitle string
	prCreateBody  string
	prCreateHead  string
	prCreateBase  string
	prCreateDraft bool
)

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Open a pull request",
	Example: `  # PR from the current branch into main
  ghub pr create --title "Add widget" --body "Closes #42"

  # Explicit refs and target repo
  ghub pr create --repo acme/widget --head my-fork:feat --base main \
      --title "Fix bug" --draft

  # Preview without opening
  ghub pr create --title "Fix bug" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if prCreateTitle == "" {
			return output.Errorf(2, "usage", "--title is required")
		}
		owner, repo, err := resolveRepo(prCreateRepo)
		if err != nil {
			return err
		}
		head := prCreateHead
		if head == "" {
			head = currentBranch()
			if head == "" {
				return output.ErrorfHint(2, "usage",
					"pass --head <branch>",
					"could not detect current branch and no --head given")
			}
		}
		base := prCreateBase
		if base == "" {
			base = "main"
		}
		opts := api.CreatePROpts{
			Owner: owner, Repo: repo,
			Title: prCreateTitle, Body: prCreateBody,
			Head: head, Base: base, Draft: prCreateDraft,
		}
		if flagDryRun {
			return output.EmitDryRun(map[string]any{"would_create": opts})
		}
		c, err := newAPIClient()
		if err != nil {
			return err
		}
		ctx, cancel := cmdContext()
		defer cancel()
		output.Progress("opening PR %s -> %s on %s/%s…", head, base, owner, repo)
		pr, err := c.CreatePR(ctx, opts)
		if err != nil {
			return err
		}
		return output.Emit(pr)
	},
}

func init() {
	prListCmd.Flags().StringVar(&prListRepo, "repo", "", "owner/name (default: detect from cwd git remote)")
	prListCmd.Flags().StringVar(&prListState, "state", "open", "open|closed|all")
	prListCmd.Flags().IntVar(&prListLimit, "limit", 25, "max items to return")
	prListCmd.Flags().StringVar(&prListCursor, "cursor", "", "pagination cursor from previous response")

	prGetCmd.Flags().StringVar(&prGetRepo, "repo", "", "owner/name (default: detect from cwd git remote)")

	prCreateCmd.Flags().StringVar(&prCreateRepo, "repo", "", "owner/name (default: detect from cwd git remote)")
	prCreateCmd.Flags().StringVar(&prCreateTitle, "title", "", "PR title (required)")
	prCreateCmd.Flags().StringVar(&prCreateBody, "body", "", "PR body / description")
	prCreateCmd.Flags().StringVar(&prCreateHead, "head", "", "source branch (default: current git branch)")
	prCreateCmd.Flags().StringVar(&prCreateBase, "base", "", "target branch (default: main)")
	prCreateCmd.Flags().BoolVar(&prCreateDraft, "draft", false, "open as draft")

	prCmd.AddCommand(prListCmd, prGetCmd, prCreateCmd)
	rootCmd.AddCommand(prCmd)
}
