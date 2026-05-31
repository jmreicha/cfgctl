package steampipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

// ---- helpers ---------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(data)
}

func basicAWSConfig() string {
	return `[default]
region = us-east-1

[profile prod-account/AdminAccess]
sso_auto_populated = true
sso_account_id = 123456789012
region = us-east-1

[profile staging-account/ReadOnly]
sso_auto_populated = true
sso_account_id = 987654321098
region = us-west-2

[sso-session cfgctl]
sso_start_url = https://example.awsapps.com/start
`
}

// ---- parseAWSProfiles ------------------------------------------------------

func TestParseAWSProfiles_Basic(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "config")
	writeFile(t, awsCfg, basicAWSConfig())

	profiles, warn, err := parseAWSProfiles(awsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
	// [default] has no sso_auto_populated, so only the two SSO profiles are returned.
	if len(profiles) != 2 {
		t.Fatalf("want 2 profiles, got %d: %v", len(profiles), profiles)
	}
}

func TestParseAWSProfiles_MissingFile(t *testing.T) {
	_, warn, err := parseAWSProfiles("/tmp/nonexistent-cfgctl-test/config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warn == "" {
		t.Error("want warning for missing file, got none")
	}
}

func TestParseAWSProfiles_SSOAutoPopulatedFilter(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "config")
	writeFile(t, awsCfg, `[default]
region = us-east-1

[profile manual-profile]
region = us-east-1

[profile sso-profile/AdminAccess]
sso_auto_populated = true
sso_account_id = 123456789012
region = us-east-1

[profile also-manual]
sso_account_id = 000000000000
`)

	profiles, warn, err := parseAWSProfiles(awsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile (only sso_auto_populated=true), got %d: %v", len(profiles), profiles)
	}
	if profiles[0] != "sso-profile/AdminAccess" {
		t.Errorf("unexpected profile: %s", profiles[0])
	}
}

func TestParseAWSProfiles_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "config")
	writeFile(t, awsCfg, "")

	profiles, warn, err := parseAWSProfiles(awsCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
	if len(profiles) != 0 {
		t.Errorf("want 0 profiles, got %d", len(profiles))
	}
}

// ---- sanitizeProfileName ---------------------------------------------------

func TestSanitizeProfileName(t *testing.T) {
	cases := []struct {
		input  string
		prefix string
		want   string
	}{
		{"prod-account/AdminAccess", "aws_", "aws_prod_account_adminaccess"},
		{"staging-account/ReadOnly", "aws_", "aws_staging_account_readonly"},
		{"default", "aws_", "aws_default"},
		{"my.profile name", "conn_", "conn_my_profile_name"},
	}
	for _, tc := range cases {
		got := sanitizeProfileName(tc.input, tc.prefix)
		if got != tc.want {
			t.Errorf("sanitizeProfileName(%q, %q) = %q, want %q", tc.input, tc.prefix, got, tc.want)
		}
	}
}

// ---- generateConnectionBlock -----------------------------------------------

func TestGenerateConnectionBlock(t *testing.T) {
	block := generateConnectionBlock("prod-account", "aws_prod_account", []string{"*"}, []string{})
	if !strings.Contains(block, managedMarker) {
		t.Error("generated block missing managed marker")
	}
	if !strings.Contains(block, `connection "aws_prod_account"`) {
		t.Error("generated block missing connection name")
	}
	if !strings.Contains(block, `profile = "prod-account"`) {
		t.Error("generated block missing profile")
	}
	if !strings.Contains(block, `regions = ["*"]`) {
		t.Error("generated block missing regions")
	}
}

func TestGenerateConnectionBlock_NoIgnoreErrorCodes(t *testing.T) {
	block := generateConnectionBlock("prod", "aws_prod", []string{"*"}, []string{})
	if strings.Contains(block, "ignore_error_codes") {
		t.Error("block should not contain ignore_error_codes when not configured")
	}
}

func TestGenerateConnectionBlock_WithIgnoreErrorCodes(t *testing.T) {
	block := generateConnectionBlock("prod", "aws_prod", []string{"*"}, []string{"AccessDenied", "UnauthorizedOperation"})
	if !strings.Contains(block, "ignore_error_codes") {
		t.Error("block should contain ignore_error_codes when configured")
	}
	if !strings.Contains(block, `"AccessDenied"`) || !strings.Contains(block, `"UnauthorizedOperation"`) {
		t.Error("block missing expected error codes")
	}
}

func TestGenerateConnectionBlock_MultipleRegions(t *testing.T) {
	block := generateConnectionBlock("prod", "aws_prod", []string{"us-east-1", "us-west-2"}, []string{})
	if !strings.Contains(block, `"us-east-1"`) || !strings.Contains(block, `"us-west-2"`) {
		t.Error("generated block missing expected regions")
	}
}

// ---- resolveRegions --------------------------------------------------------

func TestResolveRegions_Default(t *testing.T) {
	regions := resolveRegions("any-profile", []string{"*"}, map[string][]string{})
	if len(regions) != 1 || regions[0] != "*" {
		t.Errorf("unexpected regions: %v", regions)
	}
}

func TestResolveRegions_PerProfileOverride(t *testing.T) {
	overrides := map[string][]string{"prod": {"us-east-1", "us-west-2"}}
	regions := resolveRegions("prod", []string{"*"}, overrides)
	if len(regions) != 2 {
		t.Errorf("expected 2 regions, got %v", regions)
	}
}

// ---- merger ----------------------------------------------------------------

func TestParseSPCBlocks_Empty(t *testing.T) {
	blocks := parseSPCBlocks("")
	if len(blocks) != 0 {
		t.Errorf("want 0 blocks, got %d", len(blocks))
	}
}

func TestParseSPCBlocks_SingleManaged(t *testing.T) {
	content := managedMarker + "\nconnection \"aws_test\" {\n  plugin = \"aws\"\n}\n"
	blocks := parseSPCBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if !blocks[0].managed {
		t.Error("block should be managed")
	}
	if blocks[0].name != "aws_test" {
		t.Errorf("unexpected name: %s", blocks[0].name)
	}
}

func TestParseSPCBlocks_MixedBlocks(t *testing.T) {
	content := readFile(t, "testdata/existing_spc_mixed.spc")
	blocks := parseSPCBlocks(content)

	var managed, user int
	for _, b := range blocks {
		if b.name == "" {
			continue
		}
		if b.managed {
			managed++
		} else {
			user++
		}
	}
	if managed != 1 {
		t.Errorf("want 1 managed block, got %d", managed)
	}
	if user != 2 {
		t.Errorf("want 2 user blocks, got %d", user)
	}
}

func TestMergeBlocks_PreservesUserBlocks(t *testing.T) {
	existing := parseSPCBlocks(readFile(t, "testdata/existing_spc_mixed.spc"))
	generated := []spcBlock{
		{
			content: generateConnectionBlock("new-profile", "aws_new_profile", []string{"*"}, []string{}),
			name:    "aws_new_profile",
			managed: true,
		},
	}

	merged := mergeBlocks(existing, generated)

	var names []string
	for _, b := range merged {
		if b.name != "" {
			names = append(names, b.name)
		}
	}

	// User blocks preserved; old managed block gone; new managed block present.
	hasUser1 := false
	hasUser2 := false
	hasOldManaged := false
	hasNewManaged := false
	for _, n := range names {
		switch n {
		case "my_local":
			hasUser1 = true
		case "aws_cross_account":
			hasUser2 = true
		case "aws_old_profile":
			hasOldManaged = true
		case "aws_new_profile":
			hasNewManaged = true
		}
	}

	if !hasUser1 || !hasUser2 {
		t.Errorf("user blocks not preserved, names: %v", names)
	}
	if hasOldManaged {
		t.Errorf("old managed block should be replaced, names: %v", names)
	}
	if !hasNewManaged {
		t.Errorf("new managed block missing, names: %v", names)
	}
}

// ---- Generate integration --------------------------------------------------

func TestGenerate_BasicGeneration(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Error("expected files to be created")
	}

	content := readFile(t, spcOut)
	if !strings.Contains(content, "aws_prod_account") {
		t.Error("expected prod connection in output")
	}
	if !strings.Contains(content, "aws_staging_account") {
		t.Error("expected staging connection in output")
	}
	// [default] has no sso_auto_populated so it is not included.
	if strings.Contains(content, "aws_default") {
		t.Error("default profile should be excluded (no sso_auto_populated)")
	}
}

func TestGenerate_MissingAWSConfig(t *testing.T) {
	dir := t.TempDir()
	spcOut := filepath.Join(dir, "aws.spc")

	cfg := DefaultConfig()
	cfg.AWSConfigPath = filepath.Join(dir, "nonexistent")
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for missing AWS config")
	}
	if len(result.FilesCreated) != 0 {
		t.Error("should not create files when AWS config is missing")
	}
}

func TestGenerate_ProfileFiltering(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut
	cfg.Profiles = []string{"prod-account/AdminAccess"}

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content := readFile(t, spcOut)
	if !strings.Contains(content, "aws_prod_account") {
		t.Error("expected prod connection")
	}
	if strings.Contains(content, "aws_staging") {
		t.Error("staging should be filtered out")
	}

	_ = result
}

func TestGenerate_PerProfileRegions(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut
	cfg.ProfileRegions = map[string][]string{
		"prod-account/AdminAccess": {"us-east-1", "us-west-2"},
	}

	p := NewProvider(cfg)
	_, err := p.Generate(context.Background(), &core.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content := readFile(t, spcOut)
	if !strings.Contains(content, `"us-east-1"`) || !strings.Contains(content, `"us-west-2"`) {
		t.Error("expected per-profile region overrides in output")
	}
}

func TestGenerate_DryRun(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.FilesCreated) != 1 {
		t.Errorf("dry-run should report target file, got %v", result.FilesCreated)
	}
	if _, exists := result.Metadata["config_content"]; !exists {
		t.Error("dry-run should include config_content in metadata")
	}
}

func TestGenerate_PreservesUserBlocks(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())

	// Pre-populate the output with mixed content.
	existing, err := os.ReadFile("testdata/existing_spc_mixed.spc")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	writeFile(t, spcOut, string(existing))

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	_, err = p.Generate(context.Background(), &core.GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content := readFile(t, spcOut)

	// User blocks should be preserved.
	if !strings.Contains(content, `connection "my_local"`) {
		t.Error("user block my_local should be preserved")
	}
	if !strings.Contains(content, `connection "aws_cross_account"`) {
		t.Error("user block aws_cross_account should be preserved")
	}

	// Old managed block should be replaced.
	if strings.Contains(content, `connection "aws_old_profile"`) {
		t.Error("old managed block should be replaced")
	}

	// New managed blocks should be present.
	if !strings.Contains(content, "aws_prod_account") {
		t.Error("new prod managed block should be generated")
	}
}

func TestIsAggregatorBlock(t *testing.T) {
	agg := `connection "aws_all" {
  type        = "aggregator"
  plugin      = "aws"
  connections = ["aws_*"]
}`
	conn := `connection "aws_prod" {
  plugin  = "aws"
  profile = "prod"
  regions = ["*"]
}`
	if !isAggregatorBlock(agg) {
		t.Error("expected aggregator block to be detected")
	}
	if isAggregatorBlock(conn) {
		t.Error("expected connection block to not be detected as aggregator")
	}
}

func TestMergeBlocks_AggregatorAlwaysPreserved(t *testing.T) {
	// Even if an aggregator's name collides with a generated connection name,
	// it must be preserved unchanged.
	existingContent := `connection "aws_prod" {
  type        = "aggregator"
  plugin      = "aws"
  connections = ["aws_*prod*"]
}
`
	existing := parseSPCBlocks(existingContent)
	generated := []spcBlock{
		{
			// Same name as the aggregator — should NOT drop the aggregator.
			content: generateConnectionBlock("prod/cloudinfra", "aws_prod", []string{"*"}, []string{}),
			name:    "aws_prod",
			managed: true,
		},
	}

	merged := mergeBlocks(existing, generated)

	var aggCount, connCount int
	for _, b := range merged {
		if b.name != "aws_prod" {
			continue
		}
		if isAggregatorBlock(b.content) {
			aggCount++
		} else {
			connCount++
		}
	}

	if aggCount != 1 {
		t.Errorf("expected aggregator to be preserved, aggCount=%d connCount=%d", aggCount, connCount)
	}
	if connCount != 1 {
		t.Errorf("expected generated connection to also be present, aggCount=%d connCount=%d", aggCount, connCount)
	}
}

func TestExtractProfileValue(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{`connection "x" {\n  profile = "AFT-Management"\n}`, "AFT-Management"},
		{`connection "x" {\n  profile = "aft-management/cloudinfra"\n}`, "aft-management/cloudinfra"},
		{`connection "x" {\n  type = "aggregator"\n}`, ""},
	}
	for _, tc := range cases {
		got := extractProfileValue(strings.ReplaceAll(tc.content, `\n`, "\n"))
		if got != tc.want {
			t.Errorf("extractProfileValue(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}

func TestSanitizedProfileAccount(t *testing.T) {
	cases := []struct {
		profile string
		want    string
	}{
		{"AFT-Management", "aft_management"},
		{"aft-management/cloudinfra", "aft_management"},
		{"aft-management/lytxread", "aft_management"},
		{"Log", "log"},
		{"log archive/cloudinfra", "log_archive"},
		{"AML-IN", "aml_in"},
		{"aml-in/cloudinfra", "aml_in"},
	}
	for _, tc := range cases {
		got := sanitizedProfileAccount(tc.profile)
		if got != tc.want {
			t.Errorf("sanitizedProfileAccount(%q) = %q, want %q", tc.profile, got, tc.want)
		}
	}
}

func TestMergeBlocks_AccountNameDedup(t *testing.T) {
	// User block uses old-style profile (no role suffix); generated uses SSO
	// style. Same account — user block should be dropped.
	existingContent := `connection "aws_aft_management" {
  plugin  = "aws"
  profile = "AFT-Management"
  regions = ["us-east-1"]
}

connection "aws_log" {
  plugin  = "aws"
  profile = "Log"
  regions = ["us-east-1"]
}
`
	existing := parseSPCBlocks(existingContent)
	// After deduplication, one connection per account.
	generated := []spcBlock{
		{
			content: generateConnectionBlock("aft-management/cloudinfra", "aws_aft_management", []string{"*"}, []string{}),
			name:    "aws_aft_management",
			managed: true,
		},
		// "log archive/cloudinfra" has account "log archive" → "log_archive",
		// which does NOT match "Log" → "log", so aws_log should be preserved.
		{
			content: generateConnectionBlock("log archive/cloudinfra", "aws_log_archive", []string{"*"}, []string{}),
			name:    "aws_log_archive",
			managed: true,
		},
	}

	merged := mergeBlocks(existing, generated)

	var names []string
	for _, b := range merged {
		if b.name != "" {
			names = append(names, b.name)
		}
	}

	// Old aws_aft_management user block dropped (same account as generated).
	count := 0
	for _, n := range names {
		if n == "aws_aft_management" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected aws_aft_management exactly once, got %d: %v", count, names)
	}

	// aws_log should be preserved (different account from log_archive).
	hasLog := false
	for _, n := range names {
		if n == "aws_log" {
			hasLog = true
		}
	}
	if !hasLog {
		t.Errorf("aws_log should be preserved (different account), names: %v", names)
	}

	// Generated blocks should all be present.
	want := []string{"aws_aft_management", "aws_log_archive"}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s in output, names: %v", w, names)
		}
	}
}

func TestConnectionNameForProfile(t *testing.T) {
	cases := []struct {
		profile string
		prefix  string
		want    string
	}{
		{"prod-account/AdminAccess", "aws_", "aws_prod_account"},
		{"staging-account/ReadOnly", "aws_", "aws_staging_account"},
		{"default", "aws_", "aws_default"},
		{"deviceplatform/cloudinfra", "aws_", "aws_deviceplatform"},
		{"deviceplatform/lytxread", "aws_", "aws_deviceplatform"},
	}
	for _, tc := range cases {
		got := connectionNameForProfile(tc.profile, tc.prefix)
		if got != tc.want {
			t.Errorf("connectionNameForProfile(%q, %q) = %q, want %q", tc.profile, tc.prefix, got, tc.want)
		}
	}
}

func TestDedupeByAccount_SingleRole(t *testing.T) {
	profiles := []string{"default", "bedrock-test"}
	got := dedupeByAccount(profiles, nil)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d: %v", len(got), got)
	}
}

func TestDedupeByAccount_MultipleRoles(t *testing.T) {
	profiles := []string{
		"deviceplatform/cloudinfra",
		"deviceplatform/lytxread",
		"aft-management/cloudinfra",
		"aft-management/lytxread",
	}
	got := dedupeByAccount(profiles, nil)
	if len(got) != 2 {
		t.Fatalf("want 2 accounts, got %d: %v", len(got), got)
	}
	// First profile for each account used by default.
	if got[0] != "deviceplatform/cloudinfra" {
		t.Errorf("want deviceplatform/cloudinfra, got %s", got[0])
	}
	if got[1] != "aft-management/cloudinfra" {
		t.Errorf("want aft-management/cloudinfra, got %s", got[1])
	}
}

func TestDedupeByAccount_PreferredRole(t *testing.T) {
	profiles := []string{
		"deviceplatform/cloudinfra",
		"deviceplatform/lytxread",
	}
	got := dedupeByAccount(profiles, []string{"lytxread"})
	if len(got) != 1 {
		t.Fatalf("want 1, got %d: %v", len(got), got)
	}
	if got[0] != "deviceplatform/lytxread" {
		t.Errorf("want deviceplatform/lytxread, got %s", got[0])
	}
}

func TestDedupeByAccount_PreserveOrder(t *testing.T) {
	profiles := []string{
		"aft-management/cloudinfra",
		"deviceplatform/cloudinfra",
		"aft-management/lytxread",
	}
	got := dedupeByAccount(profiles, nil)
	// aft-management first, then deviceplatform.
	if len(got) != 2 || got[0] != "aft-management/cloudinfra" || got[1] != "deviceplatform/cloudinfra" {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestMergeBlocks_GeneratedWinsOnNameCollision(t *testing.T) {
	// Simulate an existing file where a user block has the same connection name
	// as a to-be-generated block — the generated block should take precedence.
	existingContent := `connection "aws_prod_account" {
  plugin  = "aws"
  profile = "prod-account/AdminAccess"
  regions = ["us-east-1"]
}

connection "my_custom" {
  plugin = "aws"
  regions = ["*"]
}
`
	existing := parseSPCBlocks(existingContent)
	generated := []spcBlock{
		{
			content: generateConnectionBlock("prod-account/AdminAccess", "aws_prod_account", []string{"*"}, []string{}),
			name:    "aws_prod_account",
			managed: true,
		},
	}

	merged := mergeBlocks(existing, generated)

	var names []string
	for _, b := range merged {
		if b.name != "" {
			names = append(names, b.name)
		}
	}

	count := 0
	for _, n := range names {
		if n == "aws_prod_account" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected aws_prod_account exactly once, got %d: %v", count, names)
	}

	for _, b := range merged {
		if b.name == "aws_prod_account" && !b.managed {
			t.Error("expected the generated (managed) block to win, got user block")
		}
	}

	hasCustom := false
	for _, n := range names {
		if n == "my_custom" {
			hasCustom = true
		}
	}
	if !hasCustom {
		t.Error("unrelated user block my_custom should be preserved")
	}
}

func TestGenerate_SkipsWhenFileExistsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())
	writeFile(t, spcOut, "# existing\n")

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{Force: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FilesSkipped) == 0 {
		t.Error("expected file to be skipped")
	}
	if len(result.FilesCreated) != 0 {
		t.Error("should not create files without --force")
	}
	if readFile(t, spcOut) != "# existing\n" {
		t.Error("existing file should be unchanged")
	}
}

func TestGenerate_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	awsCfg := filepath.Join(dir, "aws_config")
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, awsCfg, basicAWSConfig())
	writeFile(t, spcOut, "# existing\n")

	cfg := DefaultConfig()
	cfg.AWSConfigPath = awsCfg
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FilesCreated) == 0 {
		t.Error("expected file to be created with --force")
	}
}

func TestGenerate_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false

	p := NewProvider(cfg)
	result, err := p.Generate(context.Background(), &core.GenerateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.FilesCreated) != 0 {
		t.Error("disabled provider should not create files")
	}
	if len(result.Warnings) == 0 {
		t.Error("expected disabled warning")
	}
}

func TestNeedsBackup_NoForce(t *testing.T) {
	dir := t.TempDir()
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, spcOut, "# existing content\n")

	cfg := DefaultConfig()
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	needs, err := p.NeedsBackup(&core.GenerateOptions{Force: false})
	if err != nil {
		t.Fatalf("NeedsBackup: %v", err)
	}
	if needs {
		t.Error("expected NeedsBackup=false when file exists but --force not set")
	}
}

func TestNeedsBackup_Force(t *testing.T) {
	dir := t.TempDir()
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, spcOut, "# existing content\n")

	cfg := DefaultConfig()
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	needs, err := p.NeedsBackup(&core.GenerateOptions{Force: true})
	if err != nil {
		t.Fatalf("NeedsBackup: %v", err)
	}
	if !needs {
		t.Error("expected NeedsBackup=true when file exists and --force is set")
	}
}

func TestNeedsBackup_DryRun(t *testing.T) {
	dir := t.TempDir()
	spcOut := filepath.Join(dir, "aws.spc")
	writeFile(t, spcOut, "# existing content\n")

	cfg := DefaultConfig()
	cfg.ConfigPath = spcOut

	p := NewProvider(cfg)
	needs, err := p.NeedsBackup(&core.GenerateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("NeedsBackup: %v", err)
	}
	if needs {
		t.Error("expected NeedsBackup=false for dry-run")
	}
}

func TestGenerate_ReplaceDropsManualBlocks(t *testing.T) {
	awsConfig := `[profile test-account/ReadOnly]
sso_session = cfgctl
sso_account_id = 123456789012
sso_account_name = test-account
sso_role_name = ReadOnly
sso_auto_populated = true
`
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

	tests := []struct {
		name  string
		force bool
	}{
		{"replace without force", false},
		{"replace with force", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			awsDir := t.TempDir()
			spcDir := t.TempDir()

			awsConfigPath := filepath.Join(awsDir, "config")
			if err := os.WriteFile(awsConfigPath, []byte(awsConfig), 0600); err != nil {
				t.Fatalf("write aws config: %v", err)
			}

			spcPath := filepath.Join(spcDir, "aws.spc")
			if err := os.WriteFile(spcPath, []byte(existingSPC), 0600); err != nil {
				t.Fatalf("write spc: %v", err)
			}

			cfg := DefaultConfig()
			cfg.ConfigPath = spcPath
			cfg.AWSConfigPath = awsConfigPath

			p := NewProvider(cfg)
			result, err := p.Generate(context.Background(), &core.GenerateOptions{Force: tc.force, Replace: true})
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if len(result.FilesCreated) == 0 {
				t.Fatal("expected files created; Replace:true should not skip existing file")
			}

			written, err := os.ReadFile(spcPath)
			if err != nil {
				t.Fatalf("read spc: %v", err)
			}
			if strings.Contains(string(written), "manual_block") {
				t.Fatalf("manual_block should have been dropped, got:\n%s", written)
			}
		})
	}
}
