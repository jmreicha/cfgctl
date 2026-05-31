# `--replace` Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add a `--replace` flag to `cfgctl generate` that tells all supporting providers to write only cfgctl-managed content, discarding any manually-added entries in the existing config file.

**Architecture:** `Replace bool` is added to `core.GenerateOptions` and `core.ExecuteOptions`, wired through the engine, and exposed as a CLI flag. Each provider checks `opts.Replace` to bypass its preservation/merge logic: AWS skips `mergeConfigContent`, Steampipe skips `mergeBlocks`, SSH starts from a fresh config instead of parsing the existing file.

**Tech Stack:** Go, cobra (CLI flags), standard library

---

### Task 1: Add `Replace` to core types

**Files:**

- Modify: `internal/core/provider.go` (add field to `GenerateOptions`)
- Modify: `internal/core/engine.go` (add field to `ExecuteOptions`, pass through in `generateProvider`)

- [x] **Step 1: Add `Replace` to `GenerateOptions`**

In `internal/core/provider.go`, add after the `Force` field (line 74):

```go
// Replace indicates whether to discard existing manually-added content and
// write only cfgctl-managed entries. Providers that support preservation
// (AWS, Steampipe, SSH) skip their merge logic when this is true.
Replace bool
```

- [x] **Step 2: Add `Replace` to `ExecuteOptions`**

In `internal/core/engine.go`, add after the `Force` field (around line 57):

```go
// Replace discards manually-added content in existing configs and writes
// only cfgctl-managed entries.
Replace bool
```

- [x] **Step 3: Pass `Replace` through `generateProvider`**

In `internal/core/engine.go`, update `generateProvider` (around line 300) to include `Replace`:

```go
genOpts := &GenerateOptions{
    DryRun:  opts.DryRun,
    Force:   opts.Force,
    Replace: opts.Replace,
    Verbose: opts.Verbose,
    Config:  providerCfg,
    Status:  e.status,
}
```

- [x] **Step 4: Commit**

```bash
git add internal/core/provider.go internal/core/engine.go
git commit -m "feat(core): add Replace field to GenerateOptions and ExecuteOptions"
```

---

### Task 2: Wire `--replace` CLI flag

**Files:**

- Modify: `internal/cli/generate.go`

- [x] **Step 1: Declare the flag variable and wire it**

In `internal/cli/generate.go`, add `var replace bool` alongside the existing `var force bool` at the top of `newGenerateCmd`:

```go
var force bool
var replace bool
```

Then in the `opts` construction (around line 140), add `Replace: replace`:

```go
opts := &core.ExecuteOptions{
    Providers: providers,
    DryRun:    dryRun,
    Force:     force,
    Replace:   replace,
    NoBackup:  noBackup,
    Verbose:   verbose,
}
```

Then register the flag at the bottom of `newGenerateCmd` alongside `--force`:

```go
cmd.Flags().BoolVar(&replace, "replace", false, "discard manually-added entries and write only cfgctl-managed content")
```

- [x] **Step 2: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [x] **Step 3: Commit**

```bash
git add internal/cli/generate.go
git commit -m "feat(cli): add --replace flag to generate command"
```

---

### Task 3: AWS provider — skip merge when `--replace`

**Files:**

- Modify: `internal/providers/aws/provider.go`
- Test: `internal/providers/aws/provider_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/providers/aws/provider_test.go`:

```go
func TestProviderGenerateReplaceDropsManualProfiles(t *testing.T) {
	cacheDir := t.TempDir()
	configDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.ConfigPath = filepath.Join(configDir, "config")
	cfg.SSO.Region = "us-east-1"
	cfg.SSO.StartURL = "https://example.awsapps.com/start"
	cfg.TokenCachePaths = []string{cacheDir}
	cfg.Prune = true

	// Write an existing config that has a manual profile (no marker key).
	existing := "[profile manual-profile]\nregion = us-west-2\n"
	if err := os.WriteFile(cfg.ConfigPath, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	provider := NewProvider(cfg)
	provider.discover = func(_ context.Context, _ *Config) ([]DiscoveredProfile, error) {
		return []DiscoveredProfile{
			{AccountID: "123456789012", AccountName: "test-account", RoleName: "ReadOnly", SSORegion: "us-east-1"},
		}, nil
	}

	result, err := provider.Generate(context.Background(), &core.GenerateOptions{
		Force:   true,
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Fatal("expected files created")
	}

	written, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(written), "manual-profile") {
		t.Fatalf("manual-profile should have been dropped, got:\n%s", written)
	}
}
```

- [x] **Step 2: Run it to verify it fails**

```bash
go test ./internal/providers/aws/... -run TestProviderGenerateReplaceDropsManualProfiles -v
```

Expected: FAIL — manual-profile survives because `--replace` is not yet checked.

- [x] **Step 3: Update `buildConfigContent` to accept and respect `opts.Replace`**

Change the function signature in `internal/providers/aws/provider.go` from:

```go
func buildConfigContent(cfg *Config, outputPath string, profiles []DiscoveredProfile, result *core.Result) (string, []string, error) {
```

to:

```go
func buildConfigContent(cfg *Config, outputPath string, profiles []DiscoveredProfile, result *core.Result, opts *core.GenerateOptions) (string, []string, error) {
```

Inside the function, change the merge branch:

```go
finalContent := configContent
if cfg.Prune && (opts == nil || !opts.Replace) {
    mergedContent, err := mergeConfigContent(outputPath, configContent, generatedNames, cfg.MarkerKey, cfg.SSO.SessionName)
    if err != nil {
        return "", nil, err
    }
    finalContent = mergedContent
}
```

Update the call site in `Generate` (around line 91):

```go
finalContent, _, err := buildConfigContent(p.config, outputPath, profiles, result, opts)
```

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./internal/providers/aws/... -run TestProviderGenerateReplaceDropsManualProfiles -v
```

Expected: PASS

- [x] **Step 5: Run full AWS test suite**

```bash
go test ./internal/providers/aws/... -v
```

Expected: all tests pass.

- [x] **Step 6: Commit**

```bash
git add internal/providers/aws/provider.go internal/providers/aws/provider_test.go
git commit -m "feat(aws): skip mergeConfigContent when --replace is set"
```

---

### Task 4: Steampipe provider — skip merge when `--replace`

**Files:**

- Modify: `internal/providers/steampipe/provider.go`
- Test: `internal/providers/steampipe/provider_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/providers/steampipe/provider_test.go`:

```go
func TestGenerate_ReplaceDropsManualBlocks(t *testing.T) {
	awsDir := t.TempDir()
	spcDir := t.TempDir()

	// Write an AWS config with one managed profile.
	awsConfig := `[profile test-account/ReadOnly]
sso_session = cfgctl
sso_account_id = 123456789012
sso_account_name = test-account
sso_role_name = ReadOnly
sso_auto_populated = true
`
	awsConfigPath := filepath.Join(awsDir, "config")
	if err := os.WriteFile(awsConfigPath, []byte(awsConfig), 0600); err != nil {
		t.Fatalf("write aws config: %v", err)
	}

	// Write an existing SPC file with a manual (unmanaged) block.
	spcPath := filepath.Join(spcDir, "aws.spc")
	existingSPC := `# managed-by: cfgctl
connection "old_managed" {
  plugin  = "aws"
  profile = "test-account/ReadOnly"
}

connection "manual_block" {
  plugin  = "aws"
  profile = "some-other-account/Admin"
}
`
	if err := os.WriteFile(spcPath, []byte(existingSPC), 0600); err != nil {
		t.Fatalf("write spc: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = spcPath
	cfg.AWSConfigPath = awsConfigPath

	p := NewProvider(cfg)
	_, err := p.Generate(context.Background(), &core.GenerateOptions{
		Force:   true,
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	written, err := os.ReadFile(spcPath)
	if err != nil {
		t.Fatalf("read spc: %v", err)
	}
	if strings.Contains(string(written), "manual_block") {
		t.Fatalf("manual_block should have been dropped, got:\n%s", written)
	}
}
```

- [x] **Step 2: Run it to verify it fails**

```bash
go test ./internal/providers/steampipe/... -run TestGenerate_ReplaceDropsManualBlocks -v
```

Expected: FAIL — manual_block survives.

- [x] **Step 3: Update steampipe `Generate` to skip `mergeBlocks` when `opts.Replace`**

In `internal/providers/steampipe/provider.go`, change the merge block (around line 118):

```go
var finalBlocks []spcBlock
existingContent, readErr := readFileIfExists(outputPath)
switch {
case readErr != nil:
    result.Warnings = append(result.Warnings, fmt.Sprintf("failed to parse existing config, user blocks may not be preserved: %v", readErr))
    finalBlocks = generated
case existingContent == "" || (opts != nil && opts.Replace):
    finalBlocks = generated
default:
    existingBlocks := parseSPCBlocks(existingContent)
    finalBlocks = mergeBlocks(existingBlocks, generated)
}
```

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./internal/providers/steampipe/... -run TestGenerate_ReplaceDropsManualBlocks -v
```

Expected: PASS

- [x] **Step 5: Run full steampipe test suite**

```bash
go test ./internal/providers/steampipe/... -v
```

Expected: all tests pass.

- [x] **Step 6: Commit**

```bash
git add internal/providers/steampipe/provider.go internal/providers/steampipe/provider_test.go
git commit -m "feat(steampipe): skip mergeBlocks when --replace is set"
```

---

### Task 5: SSH provider — start fresh when `--replace`

**Files:**

- Modify: `internal/providers/ssh/provider.go`
- Test: `internal/providers/ssh/provider_test.go`

- [x] **Step 1: Write the failing test**

Add to `internal/providers/ssh/provider_test.go`:

```go
func TestGenerateReplaceDropsManualHosts(t *testing.T) {
	ctx := context.Background()
	sshDir := t.TempDir()
	configPath := filepath.Join(sshDir, "config")

	// Write an existing SSH config with a manual host not in cfgctl config.
	existing := `Host manual-host
  HostName 10.0.0.99
  User admin
`
	if err := os.WriteFile(configPath, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ConfigPath = sshDir
	cfg.Hosts = []HostConfig{
		{Host: "managed-host", HostName: "10.0.0.1", User: "ubuntu"},
	}

	provider := NewProvider(cfg)
	opts := &core.GenerateOptions{
		Force:   true,
		Replace: true,
	}
	result, err := provider.Generate(ctx, opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Fatal("expected files created")
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(written), "manual-host") {
		t.Fatalf("manual-host should have been dropped, got:\n%s", written)
	}
	if !strings.Contains(string(written), "managed-host") {
		t.Fatalf("managed-host should be present, got:\n%s", written)
	}
}
```

- [x] **Step 2: Run it to verify it fails**

```bash
go test ./internal/providers/ssh/... -run TestGenerateReplaceDropsManualHosts -v
```

Expected: FAIL — manual-host survives because the existing config is parsed and merged.

- [x] **Step 3: Update SSH `Generate` to use a fresh config when `opts.Replace`**

In `internal/providers/ssh/provider.go`, change the parse-or-create block (around line 151):

```go
var cfg *ssh_config.Config
_, statErr := os.Stat(configPath)
if statErr == nil && (opts == nil || !opts.Replace) {
    var err error
    cfg, err = ParseConfig(configPath)
    if err != nil {
        return nil, fmt.Errorf("failed to parse existing config: %w", err)
    }
} else {
    cfg = &ssh_config.Config{}
}
```

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./internal/providers/ssh/... -run TestGenerateReplaceDropsManualHosts -v
```

Expected: PASS

- [x] **Step 5: Run full SSH test suite**

```bash
go test ./internal/providers/ssh/... -v
```

Expected: all tests pass.

- [x] **Step 6: Commit**

```bash
git add internal/providers/ssh/provider.go internal/providers/ssh/provider_test.go
git commit -m "feat(ssh): use fresh config when --replace is set"
```

---

### Task 6: Final verification

- [x] **Step 1: Run all tests**

```bash
go test ./...
```

Expected: all tests pass.

- [x] **Step 2: Verify the flag appears in help output**

```bash
go run . generate --help
```

Expected: `--replace` appears with description "discard manually-added entries and write only cfgctl-managed content".

- [x] **Step 3: Build release binary**

```bash
go build -o cfgctl .
```

Expected: no errors.
