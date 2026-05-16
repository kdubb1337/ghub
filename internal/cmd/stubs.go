package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kdubb1337/ghub/internal/auth"
	"github.com/kdubb1337/ghub/internal/output"
)

// Wires the four Rung-3 floor commands:
//   - doctor          : health check across config + creds + API
//   - agent-context   : versioned structured introspection
//   - profile         : list (save/use/show/delete are TODO stubs)
//   - auth            : add / list / use / remove

// --- doctor -----------------------------------------------------------------

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Short:   "Health check: credentials and GitHub API reachability",
	Example: `  ghub doctor --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		report := struct {
			OK     bool          `json:"ok"`
			Checks []doctorCheck `json:"checks"`
		}{OK: true}

		// Account resolution.
		account := flagAccount
		if account == "" {
			if def, err := auth.DefaultAccount(); err == nil {
				account = def
			}
		}
		switch {
		case os.Getenv("GITHUB_TOKEN") != "":
			report.Checks = append(report.Checks, doctorCheck{
				Name: "credentials", Status: "ok", Detail: "GITHUB_TOKEN env in use",
			})
		case account == "":
			report.OK = false
			report.Checks = append(report.Checks, doctorCheck{
				Name: "credentials", Status: "missing",
				Detail: "no account configured; run `ghub auth add <id>`",
			})
		default:
			if _, err := auth.Token(account); err != nil {
				report.OK = false
				report.Checks = append(report.Checks, doctorCheck{
					Name: "credentials", Status: "missing",
					Detail: fmt.Sprintf("account %q: %v", account, err),
				})
			} else {
				report.Checks = append(report.Checks, doctorCheck{
					Name: "credentials", Status: "ok",
					Detail: fmt.Sprintf("account %q", account),
				})
			}
		}

		// API reachability: only attempt if creds exist.
		if report.OK {
			c, err := newAPIClient()
			if err != nil {
				report.OK = false
				report.Checks = append(report.Checks, doctorCheck{
					Name: "api_reachable", Status: "fail", Detail: err.Error(),
				})
			} else {
				ctx, cancel := cmdContext()
				defer cancel()
				_, _, err := c.Do(ctx, "GET", "/user", nil)
				if err != nil {
					report.OK = false
					report.Checks = append(report.Checks, doctorCheck{
						Name: "api_reachable", Status: "fail", Detail: err.Error(),
					})
				} else {
					report.Checks = append(report.Checks, doctorCheck{
						Name: "api_reachable", Status: "ok",
					})
				}
			}
		}

		if err := output.Emit(report); err != nil {
			return err
		}
		if !report.OK {
			return &output.CLIError{Code: "doctor_failed", Message: "one or more checks failed", Exit: 1}
		}
		return nil
	},
}

// --- agent-context ----------------------------------------------------------

// SchemaVersion is bumped on any breaking change to the agent-context output shape.
const SchemaVersion = 1

var agentContextCmd = &cobra.Command{
	Use:   "agent-context",
	Short: "Emit versioned structured introspection for AI agents",
	Long: `Emits a JSON document describing all commands, flags, enums, profiles,
and exit codes. Agents read this once instead of crawling --help.

Bumps schema_version on breaking shape changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := buildAgentContext(rootCmd)
		out, err := json.MarshalIndent(ctx, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

func buildAgentContext(root *cobra.Command) map[string]any {
	return map[string]any{
		"schema_version": SchemaVersion,
		"cli":            root.Name(),
		"version":        version,
		"exit_codes": map[string]int{
			"ok": 0, "generic": 1, "usage": 2, "not_found": 3, "auth": 4,
			"api": 5, "conflict": 6, "rate_limit": 7, "network": 8,
			"validation": 9, "timeout": 124,
		},
		"env": map[string]string{
			"GITHUB_TOKEN": "PAT override; takes precedence over keychain",
			"GHUB_ACCOUNT": "default account id when --account is absent",
			"GHUB_API_URL": "override api.github.com (GHES)",
		},
		"commands": describeCommands(root),
	}
}

func describeCommands(c *cobra.Command) []map[string]any {
	subs := c.Commands()
	out := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		if sub.Hidden || !sub.IsAvailableCommand() {
			continue
		}
		flags := []map[string]any{}
		sub.LocalFlags().VisitAll(func(f *pflag.Flag) {
			flags = append(flags, map[string]any{
				"name":    f.Name,
				"type":    f.Value.Type(),
				"default": f.DefValue,
				"usage":   f.Usage,
			})
		})
		entry := map[string]any{
			"name":     sub.Name(),
			"use":      sub.Use,
			"short":    sub.Short,
			"example":  sub.Example,
			"flags":    flags,
			"children": describeCommands(sub),
		}
		out = append(out, entry)
	}
	return out
}

// --- profile ----------------------------------------------------------------

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named configuration profiles",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		return output.Emit(map[string]any{
			"profiles": []string{},
			"hint":     "no profiles saved; profiles are not yet implemented",
		})
	},
}

// --- auth -------------------------------------------------------------------

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage GitHub credentials and accounts",
	Long: `Tokens are stored in the OS keychain under service "ghub".
The active account ID is recorded in the same store under "__default__".

GITHUB_TOKEN, if set, overrides the keychain entirely.`,
}

var (
	authAddToken     string
	authAddNoDefault bool
)

var authAddCmd = &cobra.Command{
	Use:   "add <account-id>",
	Short: "Store a personal access token for the given account",
	Args:  cobra.ExactArgs(1),
	Example: `  # Read token from stdin (preferred for scripts)
  echo $TOKEN | ghub auth add personal

  # Pass inline (avoid; visible in shell history)
  ghub auth add work --token ghp_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		account := args[0]
		tok := strings.TrimSpace(authAddToken)
		if tok == "" {
			// Non-interactive boundary: only read stdin if it's piped.
			fi, _ := os.Stdin.Stat()
			if (fi.Mode() & os.ModeCharDevice) == 0 {
				b, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				tok = strings.TrimSpace(b)
			}
		}
		if tok == "" {
			return output.ErrorfHint(2, "usage",
				"pass --token <pat> or pipe the token on stdin",
				"no token provided")
		}
		if err := auth.Set(account, tok); err != nil {
			return output.Errorf(1, "keychain_write_failed", "store token: %v", err)
		}
		if !authAddNoDefault {
			if _, err := auth.DefaultAccount(); err != nil {
				_ = auth.SetDefaultAccount(account)
			}
		}
		return output.Emit(map[string]any{
			"account": account,
			"stored":  true,
		})
	},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured accounts",
	Long: `Keychain backends do not enumerate entries portably; this prints the
recorded default account only. Track the IDs you've added externally if you
need a full list.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		def, _ := auth.DefaultAccount()
		return output.Emit(map[string]any{
			"default_account": def,
			"env_override":    os.Getenv("GITHUB_TOKEN") != "",
		})
	},
}

var authUseCmd = &cobra.Command{
	Use:   "use <account-id>",
	Short: "Set the default account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		account := args[0]
		if _, err := auth.Token(account); err != nil {
			return output.Errorf(3, "not_found",
				"no token stored for account %q; run `ghub auth add %s` first", account, account)
		}
		if err := auth.SetDefaultAccount(account); err != nil {
			return output.Errorf(1, "keychain_write_failed", "set default: %v", err)
		}
		return output.Emit(map[string]any{"default_account": account})
	},
}

var authRemoveCmd = &cobra.Command{
	Use:   "remove <account-id>",
	Short: "Remove the stored token for an account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		account := args[0]
		if !flagForce && !flagYes {
			return output.ErrorfHint(2, "usage",
				"pass --force to confirm",
				"destructive: removes the stored token for %q", account)
		}
		if err := auth.Delete(account); err != nil {
			return output.Errorf(3, "not_found", "no token stored for %q: %v", account, err)
		}
		return output.Emit(map[string]any{"account": account, "removed": true})
	},
}

// --- skill-path -------------------------------------------------------------

var skillPathCmd = &cobra.Command{
	Use:   "skill-path",
	Short: "Print the absolute path to the bundled SKILL.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, "..", "share", "ghub", "skills", "ghub", "SKILL.md"),
			filepath.Join(dir, "skills", "ghub", "SKILL.md"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				abs, _ := filepath.Abs(p)
				fmt.Fprintln(os.Stdout, abs)
				return nil
			}
		}
		return output.Errorf(1, "skill_not_found",
			"could not locate bundled SKILL.md; tried %v (os=%s)", candidates, runtime.GOOS)
	},
}

func init() {
	authAddCmd.Flags().StringVar(&authAddToken, "token", "", "token value (prefer stdin to keep it out of shell history)")
	authAddCmd.Flags().BoolVar(&authAddNoDefault, "no-default", false, "do not promote this account to default if none is set")

	profileCmd.AddCommand(profileListCmd)
	authCmd.AddCommand(authAddCmd, authListCmd, authUseCmd, authRemoveCmd)
	rootCmd.AddCommand(doctorCmd, agentContextCmd, profileCmd, authCmd, skillPathCmd)
}
