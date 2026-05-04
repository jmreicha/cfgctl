# cfgctl

A command line tool to simplify the creation and management of complicated configurations.

## Why?

Imagine you are a new hire at a company and you need to set up your local
environment, but the setup process is complicated and involves multiple steps,
and everybody is busy. You need to install various tools, configure them, set up
your environment variables, etc. You also need to get connected to you cloud
environments, set Kubernetes configuration, and deal with other complicated
configs.

This setup process can be time-consuming and error-prone. Additionally, you may
not understand (or care about) security implications or know about advanced
configuration which you may want but don't know exist.

`cfgctl` aims to simplify all of this setup by providing a simple command line
interface to handle this for you so you can focus on more important things.

## Getting Started

### Prerequisites

This project requires the following tools:

- **Go** 1.23+ - [Install Go](https://go.dev/doc/install)
- **Task** - [Install Task](https://taskfile.dev/installation/)
- **golangci-lint** - Go linter
- **pre-commit** - Pre-commit hook framework
- **bd (beads)** - Issue tracker for git
- **common-repo** - Repository configuration management
- **wt (worktrunk)** (optional) - Git worktree management
- **prek** (optional) - Pre-commit hook management tool
- **mcp-cli** (optional) - MCP server interface
- **gh** (optional) - GitHub CLI

### Installation

#### Homebrew

```bash
brew tap jmreicha/tap
brew install --cask cfgctl
```

1. Clone the repository:

```bash
git clone git@github.com:jmreicha/cfgctl.git
cd cfgctl
```

2. Bootstrap the development environment:

```bash
# One command to install everything
task bootstrap
```

This will:

- Check for required tools (go, golangci-lint, bd, pre-commit)
- Auto-install missing tools (uses Homebrew on macOS)
- Install pre-commit hooks (via prek or pre-commit)
- Download and verify Go dependencies

### Usage

#### Global flags

These flags apply to all commands:

| Flag             | Env var            | Description                                           |
| ---------------- | ------------------ | ----------------------------------------------------- |
| `--config`       | `CFGCTL_CONFIG`    | Config file path (default: search standard locations) |
| `--dry-run`      | `CFGCTL_DRY_RUN`   | Simulate actions without writing files                |
| `--debug`        | `CFGCTL_DEBUG`     | Enable debug logging                                  |
| `-v / --verbose` | `CFGCTL_VERBOSE`   | Verbose provider output                               |
| `--no-backup`    | `CFGCTL_NO_BACKUP` | Skip backup creation before generation                |

Environment variables take effect when the corresponding flag is not explicitly set. Flag values always win.

AWS provider example:

```bash
# Generate AWS config with SSO (auto-triggers login if needed)
cfgctl generate aws --aws-sso-region us-west-2 --aws-sso-url https://<id>.awsapps.com/start

# Overwrite existing config
cfgctl generate aws --aws-sso-region us-west-2 --aws-sso-url https://<id>.awsapps.com/start --force

# Filter to specific roles
cfgctl generate aws --aws-sso-region us-west-2 --aws-sso-url https://<id>.awsapps.com/start --aws-roles Admin,ReadOnly

# Preview changes without writing
cfgctl generate aws --dry-run

# Use env var instead of flag
CFGCTL_DRY_RUN=true cfgctl generate aws
```

SSH provider example:

```bash
# List available providers
cfgctl list

# Preview SSH changes
cfgctl generate ssh --dry-run

# Generate SSH config
cfgctl generate ssh

# Custom SSH config directory
cfgctl generate ssh --ssh-config-path ~/.ssh

# Overwrite existing config
cfgctl generate ssh --force

# Validate config
cfgctl validate
```

Kubernetes provider example:

```bash
# List available providers
cfgctl list

# Generate kubeconfig with EKS discovery (auto-discovers clusters)
cfgctl generate kubernetes

# Generate for specific EKS regions
cfgctl generate kubernetes --eks-regions us-west-2,us-east-1

# Filter to specific EKS roles (matches profile names containing role)
cfgctl generate kubernetes --eks-roles admin,readonly

# Merge with existing kubeconfig files
cfgctl generate kubernetes --kube-merge

# Merge only (skip AWS discovery, just merge existing files)
cfgctl generate kubernetes --kube-merge-only

# Preview changes without writing
cfgctl generate kubernetes --dry-run

# Overwrite existing kubeconfig
cfgctl generate kubernetes --force
```

Granted provider example:

```bash
# Configure Granted for AWS vault
cfgctl generate granted
```

#### Check command

Verify that credentials and endpoints are live and working across all providers:

```bash
# Check all providers
cfgctl check

# Check specific providers
cfgctl check kubernetes aws

# Adjust per-check timeout (default 10s)
cfgctl check --timeout 30s
```

Output is a table with colored status labels:

- `OKAY` green — healthy
- `WARN` yellow — degraded (e.g. credentials expiring soon)
- `ERROR` red — failed (unreachable, expired, or missing)

The command exits non-zero if any check fails, making it suitable for automation and CI health gates.

### Working with Beads

This project uses [beads](https://github.com/steveyegge/beads) for local issue tracking. Beads is a git-backed, distributed issue tracker optimized for AI agents.

**Essential commands:**

```bash
# View ready work (no blockers)
bd ready

# Create a new issue
bd create "Task title" -p 1 -d "Description"

# Show issue details
bd show <issue-id>

# Update issue status
bd update <issue-id> --status in_progress

# Close completed issue
bd close <issue-id>

# Sync before pushing
bd sync
```

### Available Tasks

View all available tasks with:

```bash
task --list
```

Common tasks:

```bash
task check-tools     # Check if required CLI tools are installed
task install-tools   # Install missing tools and pre-commit hooks
task bootstrap       # Bootstrap and set up dependencies and tools
task build           # Build the binary
task fmt             # Format Go code
task lint            # Run linters
task test            # Run tests
task check           # Run fmt, lint, and test
task clean           # Clean build artifacts
```

### Autonomous Iteration

For autonomous development, use the iteration loop to work through beads issues:

```bash
# Run with default settings (picks the first open issue)
task iterate

# Custom max iterations
task iterate -- --max-iterations 10

# Work on specific issue
task iterate -- --issue-id <issue-id>

# With environment variable
MAX_ITERATIONS=25 task iterate

# Use different AI agent
AI_AGENT='claude' task iterate

# Adjust timeout (default 30 minutes)
OPENCODE_TIMEOUT=3600 task iterate
```

### Development Workflow

1. Clone the repository
2. Create or claim an issue
3. Create a feature branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run checks: `task check` (runs fmt, lint, and test)
6. Commit with conventional commit format: `git commit -m "feat: your feature"`
7. Push feature branch: `git push -u origin feature/your-feature`
8. Create pull request: `gh pr create --title "feat: your feature" --body "Description"`
9. Address review feedback and push updates to the PR

### Releases

Releases are automated on pushes to `main` with semantic-release and GoReleaser. The workflow uses `.releaserc.yaml` and installs the semantic-release Node.js packages at runtime.

Required secrets:

- `GITHUB_TOKEN` (provided automatically by GitHub Actions)

Optional secrets (only if configured in `.goreleaser.yml`):

- `CI_GITHUB_TOKEN` for Homebrew tap publishing
