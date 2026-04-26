# CLI Refactor: Cobra + Viper + Lipgloss

## Overview

Refactor `internal/cli` to add Viper for environment variable support, rename EKS-specific flags,
move `--ssh-config-path` off the root command, replace raw ANSI color helpers with Lipgloss, and
add provider name validation. Cobra is already in use — this is not a framework swap.

## Goals

- Add Viper for env var + config file support (matching Docker, Kubernetes, GitHub CLI patterns)
- Add Lipgloss for styled terminal output
- Rename EKS-specific flags to be explicit (`--eks-roles`, `--eks-regions`)
- Move `--ssh-config-path` from root to `generate`
- Confirm steampipe flags in scope
- Add environment variable support (`CFGCTL_*`)
- Preserve all existing global flags and commands

## Approach

Restructure `root.go` minimally, then layer changes. The file is ~320 lines and not expected to
grow significantly — no file splitting needed. The one new file is `styles.go` for Lipgloss, which
is a genuinely separate concern reused across command files.

## File Structure

```
internal/cli/
  root.go          — flags, Viper bindings, initializeComponents, CLI overrides (as today + Viper)
  styles.go        — Lipgloss style definitions (NEW, replaces colorize helpers)
  generate.go      — renamed EKS flags, ssh flag moved here from root
  clean.go         — provider validation added, output uses styles
  list.go          — output uses styles
  validate.go      — output uses styles
  version.go       — no change
  commands_test.go — updated for flag renames + new env var tests
  generate_test.go — updated for renamed flags
```

## Flag Changes

| Old flag                    | New flag                    | Location   | Notes                   |
| --------------------------- | --------------------------- | ---------- | ----------------------- |
| `--kube-regions`            | `--eks-regions`             | `generate` | EKS-specific            |
| `--kube-roles`              | `--eks-roles`               | `generate` | EKS-specific            |
| `--ssh-config-path`         | `--ssh-config-path`         | `generate` | Moved off root          |
| `--kube-merge`              | `--kube-merge`              | `generate` | Kept — not EKS-specific |
| `--kube-merge-only`         | `--kube-merge-only`         | `generate` | Kept — not EKS-specific |
| `--steampipe-ignore-errors` | `--steampipe-ignore-errors` | `generate` | Confirmed in scope      |
| `--steampipe-regions`       | `--steampipe-regions`       | `generate` | Confirmed in scope      |

All other flags (`--config`, `--dry-run`, `--debug`, `--verbose`, `--no-backup`) remain global on root.

## Viper Wiring

- `viper.SetEnvPrefix("CFGCTL")` + `viper.AutomaticEnv()` called in `PersistentPreRunE`
- Each global flag bound with `viper.BindPFlag`
- `initializeComponents()` reads from Viper, unifying flag and env var values
- Precedence: flag > env var > config file > default

**Supported env vars:**

| Env var            | Flag          |
| ------------------ | ------------- |
| `CFGCTL_CONFIG`    | `--config`    |
| `CFGCTL_DRY_RUN`   | `--dry-run`   |
| `CFGCTL_DEBUG`     | `--debug`     |
| `CFGCTL_VERBOSE`   | `--verbose`   |
| `CFGCTL_NO_BACKUP` | `--no-backup` |

## Provider Validation

`generate` and `clean` commands validate provider name arguments before execution:

```
unknown provider "foo" — valid providers: aws, granted, kubernetes, ssh, steampipe
```

Conflicting flag combinations are also validated (e.g. `--kube-merge-only` + `--eks-regions` should error).

## Lipgloss Styling

New `styles.go` replaces `colorize`, `formatLabel`, `formatPath`, `formatSection`,
`formatWarning`, `supportsColor`, and `isTerminal` in `generate.go`. Lipgloss handles
`NO_COLOR` and non-TTY detection natively.

```go
var (
    sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
    labelStyle   = lipgloss.NewStyle().Bold(true)
    pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
    warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)
```

`generate.go`, `list.go`, `validate.go`, and `clean.go` all use these styles for consistent output.

## TDD Approach

Tests are written before implementation. Each round produces failing tests; implementation makes
them pass.

### Round 0: Reconcile existing tests

Update or remove tests that conflict with the new design before writing new ones:

- `TestNewRootCmdFlags` — assert `ssh-config-path` is on `generate`, not root
- `TestApplyKubernetesCLIOverrides` — update var names to `eksRegions`/`eksRoles`
- `TestColorize` — remove; replaced by Lipgloss style tests

### Round 1: Flag renames and moves

- `TestGenerateCmdEKSFlags` — `--eks-regions`/`--eks-roles` exist on `generate`; `--kube-regions`/`--kube-roles` do not
- `TestRootCmdNoSSHFlag` — `--ssh-config-path` absent from root persistent flags
- `TestGenerateCmdSSHFlag` — `--ssh-config-path` present on `generate`

### Round 2: Viper / env vars

- `TestEnvVarDryRun` — `CFGCTL_DRY_RUN=true` sets dry-run without flag
- `TestEnvVarNoBackup`, `TestEnvVarDebug`, `TestEnvVarVerbose`, `TestEnvVarConfig` — same pattern

### Round 3: Provider validation

- `TestGenerateCmdUnknownProvider` — returns error with valid provider list
- `TestCleanCmdUnknownProvider` — same

### Round 4: Lipgloss

- `TestStylesNoColor` — `NO_COLOR=1` produces plain text output
- Styled output differs from plain text when color is active

### Existing tests confirmed unaffected

- `TestInitializeComponents`
- `TestApplyKubernetesCLIOverridesMergeOnly`
- `TestInitializeComponentsAWSCLIOverrides`
- `TestGenerateCmd` / `TestGenerateCmdAllKeyword`
- `TestPrintGenerateResults`
- All clean / list / validate / version tests

## User Stories

### US-001: Add Viper dependency

- [ ] Add `github.com/spf13/viper` to go.mod
- [ ] Run `go mod tidy` to resolve dependencies
- [ ] `go build ./...` succeeds

### US-002: Refactor root command with global flags and Viper

- [ ] Global flags: `--config`, `--dry-run`, `--debug`, `-v/--verbose`, `--no-backup`
- [ ] Environment variable support: `CFGCTL_CONFIG`, `CFGCTL_DRY_RUN`, `CFGCTL_DEBUG`, `CFGCTL_VERBOSE`, `CFGCTL_NO_BACKUP`
- [ ] Viper reads from config file, env vars, and flags (in precedence order)
- [ ] Global flags apply to all commands
- [ ] `go test ./...` passes

### US-003: Add generate command with provider flags

- [ ] `cfgctl generate aws` with AWS-specific flags (credential-process, credentials, demo, prefix, prune, roles, sso-url, sso-region, template)
- [ ] `cfgctl generate kubernetes` with kube-specific flags (merge, merge-only, eks-regions, eks-roles)
- [ ] `cfgctl generate ssh` with SSH-specific flag (ssh-config-path)
- [ ] `cfgctl generate steampipe` with steampipe-specific flags (steampipe-ignore-errors, steampipe-regions)
- [ ] Positional args work: `cfgctl generate aws kubernetes`
- [ ] `cfgctl generate all` still works
- [ ] `--force` flag on generate command
- [ ] `go test ./...` passes

### US-004: Rename EKS-specific flags

- [ ] `--kube-regions` renamed to `--eks-regions`
- [ ] `--kube-roles` renamed to `--eks-roles`
- [ ] Help text clarifies these are for EKS discovery
- [ ] `go test ./...` passes

### US-005: Add provider validation with helpful errors

- [ ] Invalid provider name errors with list of valid providers
- [ ] Conflicting flag combinations validated (`--kube-merge-only` + `--eks-regions` should error)
- [ ] `go test ./...` passes

### US-006: Add Lipgloss output styling

- [ ] Add `github.com/charmbracelet/lipgloss` to go.mod
- [ ] Create `internal/cli/styles.go` with Lipgloss style definitions
- [ ] Provider names styled (bold/color)
- [ ] Success/error/warning messages styled distinctly
- [ ] File paths styled
- [ ] Respects `NO_COLOR` env var and non-TTY output
- [ ] `go test ./...` passes

### US-007: Update list command output

- [ ] `cfgctl list` shows registered providers
- [ ] Output uses Lipgloss styling
- [ ] `go test ./...` passes

### US-008: Update clean command

- [ ] `cfgctl clean aws` removes AWS-generated configs
- [ ] `cfgctl clean kubernetes` removes kube-generated configs
- [ ] Multiple providers: `cfgctl clean aws kubernetes`
- [ ] Provider validation: unknown provider errors with valid provider list
- [ ] Output uses Lipgloss styling
- [ ] `go test ./...` passes

### US-009: Update validate command output

- [ ] `cfgctl validate` checks all provider prerequisites
- [ ] Output uses Lipgloss styling
- [ ] `go test ./...` passes

### US-010: Wire CLI to main

- [ ] All commands (generate, list, clean, validate, version) wired to root
- [ ] Viper properly initialized at startup
- [ ] All existing functionality preserved
- [ ] `go test ./...` passes

### US-011: Update tests

- [ ] Existing tests reconciled (Round 0)
- [ ] New tests written for renamed flags, env vars, provider validation, Lipgloss
- [ ] All tests pass

## Quality Gates

Every round must pass before moving to the next:

- `go test ./...`
- `golangci-lint run`
- `go build ./...`

## Out of Scope

- Interactive TUI mode
- Shell completion generation
- Config file migration
- Splitting `root.go` into multiple files (not warranted at current size)
