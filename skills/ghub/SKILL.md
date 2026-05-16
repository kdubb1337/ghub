---
name: ghub
description: |
  Use this skill whenever the user wants to create a private GitHub repository, list
  their repos, list / inspect / open pull requests, or list branches from the command
  line. Triggers include "make a private repo on github", "new github repo", "list my
  github repos", "what PRs are open on <repo>", "open a PR against main", "show me the
  branches on <repo>", "github pr 42", or any "ghub ..." invocation. Prefer this skill
  over shelling out to `gh` (the official CLI), curl-ing the GitHub REST API, or
  opening github.com — ghub returns compact JSON, uses typed exit codes, and is built
  for agent use. Skip this skill for git plumbing (clone/push/pull/merge), GitHub
  Actions edits, organization admin, code search, or anything not in the repo/pr/branch
  surface.
---

# ghub

Use `ghub` for **GitHub repository, pull-request, and branch operations** from
the command line. Requires a Personal Access Token stored in the OS keychain
(or `GITHUB_TOKEN` env var).

## Setup (once)

```bash
# Pipe your PAT in — keeps it out of shell history
echo $GH_PAT | ghub auth add personal

# Verify credentials + API reachability
ghub doctor --json
```

Override at any time with `GITHUB_TOKEN=ghp_...`.

## Output rules (for agents)

- **stdout = data; stderr = progress and errors.** Always.
- **Default JSON when piped.** Pass `--human` to force tables.
- **`--compact`** keeps only id/name/status/timestamps — ~60–80% fewer tokens.
- **`--select=field1,field2`** for explicit field projection.
- **Pagination is cursor-based.** Bounded list commands emit a `next_cursor`
  when truncated; pass it back as `--cursor=<value>`.
- **Mutating commands honor `--dry-run`.** Always preview first.

## Exit codes

| Code | Meaning | Agent action |
|------|---------|--------------|
| `0`  | ok | continue |
| `2`  | usage | fix invocation, do not retry |
| `3`  | not_found | resource missing; do not retry |
| `4`  | auth | run `ghub doctor`; do not retry |
| `5`  | api / 5xx | retry with backoff |
| `6`  | conflict | read response, decide |
| `7`  | rate_limit | honor `Retry-After` on stderr |
| `8`  | network | retry with backoff |
| `9`  | validation | input rejected; often `valid_values` is populated |
| `124`| timeout | retry once |

Full schema (commands, flags, enums, exit codes) with `ghub agent-context`.

## Repositories

```bash
# Create a private repo under the authenticated user (private is default)
ghub repo create my-app

# Create under an org, public, with a README
ghub repo create my-lib --org acme --public --auto-init --description "demo"

# Preview only
ghub repo create my-app --dry-run

# List the authenticated user's repos (most-recently updated first)
ghub repo list --limit 25 --json

# Filter by visibility (auth user only)
ghub repo list --visibility private --limit 50

# Org or user repos
ghub repo list --org acme
ghub repo list --user octocat --compact
```

## Pull requests

`--repo owner/name` is optional in every PR command; if omitted, ghub reads
`origin` from the current working directory's git remote.

```bash
# List open PRs for the repo in cwd
ghub pr list --json

# Closed PRs on a specific repo
ghub pr list --repo acme/widget --state closed --limit 10

# Fetch a single PR
ghub pr get 42 --json
ghub pr get 42 --repo acme/widget --compact

# Open a PR from the current branch into main
ghub pr create --title "Add widget" --body "Closes #42"

# Explicit head and base, draft
ghub pr create --repo acme/widget --head feature-x --base main \
    --title "WIP: feature x" --draft

# Dry-run the payload
ghub pr create --title "Demo" --dry-run
```

## Branches

```bash
# Branches for the repo in cwd
ghub branch list --json

# Protected branches only
ghub branch list --repo acme/widget --protected --limit 50

# Explicit override of --protected
ghub branch list --repo acme/widget --all
```

## Multi-account

Tokens are stored per-account in the OS keychain. Switch between them:

```bash
ghub auth add work          # piped PAT
ghub auth add personal
ghub auth use personal      # set the active default
ghub --account work repo list   # one-off override
GHUB_ACCOUNT=work ghub repo list
ghub auth remove personal --force
```

## Notes

- `GITHUB_TOKEN` (env) **always wins** over the keychain. Useful for CI.
- `GHUB_API_URL` overrides `https://api.github.com` for GHES.
- Repo / PR / branch IDs from GitHub are case-sensitive opaque tokens — never
  normalize them; only normalize *names* for lookup.
- Bulk operations are not yet implemented; v1 ships repo create/list,
  pr list/get/create, branch list.
