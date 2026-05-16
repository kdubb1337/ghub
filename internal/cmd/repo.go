package cmd

import (
	"github.com/spf13/cobra"

	"github.com/kdubb1337/ghub/internal/api"
	"github.com/kdubb1337/ghub/internal/output"
)

// `ghub repo` — repository commands (create, list).

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage GitHub repositories",
}

// --- repo create ------------------------------------------------------------

var (
	repoCreateOrg         string
	repoCreatePrivate     bool
	repoCreatePublic      bool
	repoCreateDescription string
	repoCreateInit        bool
)

var repoCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new repository (private by default)",
	Args:  cobra.ExactArgs(1),
	Example: `  # Create a private repo under the authenticated user
  ghub repo create my-app

  # Create under an org, public, with README
  ghub repo create my-lib --org acme --public --auto-init

  # Preview the payload first
  ghub repo create my-app --description "demo" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if repoCreatePrivate && repoCreatePublic {
			return output.Errorf(2, "usage", "--private and --public are mutually exclusive")
		}
		private := true // default
		if repoCreatePublic {
			private = false
		}
		opts := api.CreateRepoOpts{
			Name:        name,
			Private:     private,
			Description: repoCreateDescription,
			AutoInit:    repoCreateInit,
			Org:         repoCreateOrg,
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
		output.Progress("creating repo %s…", name)
		repo, err := c.CreateRepo(ctx, opts)
		if err != nil {
			return err
		}
		return output.Emit(repo)
	},
}

// --- repo list --------------------------------------------------------------

var (
	repoListUser       string
	repoListOrg        string
	repoListVisibility string
	repoListLimit      int
	repoListCursor     string
)

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List repositories",
	Example: `  # Authenticated user's repos (private + public), most recent first
  ghub repo list --json

  # An org's repos
  ghub repo list --org acme --limit 50

  # Only public repos for a user
  ghub repo list --user octocat --limit 10 --compact`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if repoListUser != "" && repoListOrg != "" {
			return output.Errorf(2, "usage", "--user and --org are mutually exclusive")
		}
		if repoListVisibility != "" {
			switch repoListVisibility {
			case "all", "public", "private":
			default:
				return output.ErrorfEnum(9, "validation",
					[]string{"all", "public", "private"},
					"invalid --visibility %q", repoListVisibility)
			}
		}
		c, err := newAPIClient()
		if err != nil {
			return err
		}
		ctx, cancel := cmdContext()
		defer cancel()
		repos, next, err := c.ListRepos(ctx, api.ListReposOpts{
			User:       repoListUser,
			Org:        repoListOrg,
			Visibility: repoListVisibility,
			Limit:      repoListLimit,
			Cursor:     repoListCursor,
		})
		if err != nil {
			return err
		}
		if next != "" {
			return output.EmitPage(repos, next, "more available; pass --cursor=<value>")
		}
		return output.Emit(repos)
	},
}

func init() {
	repoCreateCmd.Flags().StringVar(&repoCreateOrg, "org", "", "create under an organization instead of the authenticated user")
	repoCreateCmd.Flags().BoolVar(&repoCreatePrivate, "private", false, "create as private (default)")
	repoCreateCmd.Flags().BoolVar(&repoCreatePublic, "public", false, "create as public")
	repoCreateCmd.Flags().StringVar(&repoCreateDescription, "description", "", "repo description")
	repoCreateCmd.Flags().BoolVar(&repoCreateInit, "auto-init", false, "initialize with a README")

	repoListCmd.Flags().StringVar(&repoListUser, "user", "", "list repos owned by this user (public only)")
	repoListCmd.Flags().StringVar(&repoListOrg, "org", "", "list repos under this organization")
	repoListCmd.Flags().StringVar(&repoListVisibility, "visibility", "", "filter (auth-user only): all|public|private")
	repoListCmd.Flags().IntVar(&repoListLimit, "limit", 25, "max items to return")
	repoListCmd.Flags().StringVar(&repoListCursor, "cursor", "", "pagination cursor from previous response")

	repoCmd.AddCommand(repoCreateCmd, repoListCmd)
	rootCmd.AddCommand(repoCmd)
}
