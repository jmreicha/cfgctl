package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/jmreicha/cfgctl/internal/providers/aws"
	"github.com/jmreicha/cfgctl/internal/providers/kubernetes"
)

type commandProvider struct {
	name         string
	cleanErr     error
	validateErr  error
	cleanCalled  bool
	validChecked bool
}

func (p *commandProvider) Name() string {
	return p.name
}

func (p *commandProvider) Validate(_ context.Context) error {
	p.validChecked = true
	return p.validateErr
}

func (p *commandProvider) Generate(_ context.Context, _ *core.GenerateOptions) (*core.Result, error) {
	return &core.Result{Provider: p.name}, nil
}

func (p *commandProvider) Backup(_ context.Context) (string, error) {
	return "", nil
}

func (p *commandProvider) Restore(_ context.Context, _ string) error {
	return nil
}

func (p *commandProvider) Clean(_ context.Context) error {
	p.cleanCalled = true
	return p.cleanErr
}

func setupCommandEngine(t *testing.T, providers ...core.Provider) {
	t.Helper()

	logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	registry = core.NewRegistry()
	backupManager = core.NewBackupManager("")
	config = core.NewConfig()
	engine = core.NewEngine(registry, backupManager, config, logger)

	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("failed to register provider: %v", err)
		}
	}
}

func TestInitializeComponents(t *testing.T) {
	cfgFile = ""
	dryRun = false
	debug = false
	noBackup = false
	sshConfigPath = ""
	verbose = false

	if err := initializeComponents(); err != nil {
		t.Fatalf("initializeComponents failed: %v", err)
	}

	if registry == nil {
		t.Fatal("expected registry to be initialized")
	}

	providers := registry.List()
	if len(providers) != 5 {
		t.Fatalf("expected five providers, got %v", providers)
	}
	if providers[0] != "aws" || providers[1] != "granted" || providers[2] != "kubernetes" || providers[3] != "ssh" || providers[4] != "steampipe" {
		t.Fatalf("expected aws, granted, kubernetes, ssh, and steampipe providers, got %v", providers)
	}
}

func TestNewRootCmdFlags(t *testing.T) {
	cmd := NewRootCmd("1.0.0")
	flags := cmd.PersistentFlags()

	for _, name := range []string{"config", "debug", "dry-run", "no-backup", "verbose"} {
		if flags.Lookup(name) == nil {
			t.Fatalf("expected %s flag", name)
		}
	}
}

func TestApplyKubernetesCLIOverrides(t *testing.T) {
	prevKubeMerge := kubeMerge
	prevKubeMergeOnly := kubeMergeOnly
	prevEKSRegions := eksRegions
	defer func() {
		kubeMerge = prevKubeMerge
		kubeMergeOnly = prevKubeMergeOnly
		eksRegions = prevEKSRegions
	}()

	kubeMerge = true
	kubeMergeOnly = false
	eksRegions = "us-west-2,us-east-1"

	cfg := kubernetes.DefaultConfig()
	applyKubernetesCLIOverrides(cfg)

	if !cfg.MergeEnabled {
		t.Fatal("expected merge to be enabled")
	}
	if cfg.MergeOnly {
		t.Fatal("expected merge-only to be false")
	}
	if !reflect.DeepEqual(cfg.AWS.Regions, []string{"us-east-1", "us-west-2"}) {
		t.Fatalf("regions = %v", cfg.AWS.Regions)
	}
}

func TestApplyKubernetesCLIOverridesMergeOnly(t *testing.T) {
	prevKubeMerge := kubeMerge
	prevKubeMergeOnly := kubeMergeOnly
	defer func() {
		kubeMerge = prevKubeMerge
		kubeMergeOnly = prevKubeMergeOnly
	}()

	kubeMerge = false
	kubeMergeOnly = true

	cfg := kubernetes.DefaultConfig()
	applyKubernetesCLIOverrides(cfg)

	if !cfg.MergeOnly {
		t.Fatal("expected merge-only to be true")
	}
	if !cfg.MergeEnabled {
		t.Fatal("expected merge to be enabled")
	}
}

func TestInitializeComponentsAWSCLIOverrides(t *testing.T) {
	cfgFile = ""
	dryRun = false
	noBackup = false
	sshConfigPath = ""
	verbose = false
	awsCredentialProcess = true
	awsCredentials = true
	awsDemo = true
	awsPrefix = "team-"
	awsPrune = true
	awsRoleFilters = "AdminAccess,ReadOnly"
	awsTemplate = "{{ .account }}-{{ .role }}"

	if err := initializeComponents(); err != nil {
		t.Fatalf("initializeComponents failed: %v", err)
	}

	providerConfig := config.GetProviderConfig(aws.ProviderName)
	awsConfig, ok := providerConfig.(*aws.Config)
	if !ok {
		t.Fatalf("expected aws config, got %T", providerConfig)
	}

	if !awsConfig.UseCredentialProcess {
		t.Fatal("expected credential_process enabled")
	}
	if !awsConfig.GenerateCredentials {
		t.Fatal("expected credentials generation enabled")
	}
	if !awsConfig.Demo {
		t.Fatal("expected demo enabled")
	}
	if awsConfig.ProfilePrefix != "team-" {
		t.Fatalf("profile prefix = %q", awsConfig.ProfilePrefix)
	}
	if !awsConfig.Prune {
		t.Fatal("expected prune enabled")
	}
	if awsConfig.ProfileTemplate != "{{ .account }}-{{ .role }}" {
		t.Fatalf("profile template = %q", awsConfig.ProfileTemplate)
	}
	if !reflect.DeepEqual(awsConfig.Roles, []string{"AdminAccess", "ReadOnly"}) {
		t.Fatalf("roles = %#v", awsConfig.Roles)
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd("1.0.0")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
}

func TestListCmd_Empty(t *testing.T) {
	setupCommandEngine(t)

	cmd := newListCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list command failed: %v", err)
	}
}

func TestListCmd_WithProviders(t *testing.T) {
	setupCommandEngine(t, &commandProvider{name: "alpha"})

	cmd := newListCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list command failed: %v", err)
	}
}

func TestValidateCmd(t *testing.T) {
	provider := &commandProvider{name: "alpha"}
	setupCommandEngine(t, provider)

	cmd := newValidateCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate command failed: %v", err)
	}
	if !provider.validChecked {
		t.Fatal("expected provider validation")
	}
}

func TestValidateCmd_Error(t *testing.T) {
	provider := &commandProvider{name: "alpha", validateErr: errors.New("no")}
	setupCommandEngine(t, provider)

	cmd := newValidateCmd()
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCleanCmd_NoArgs(t *testing.T) {
	setupCommandEngine(t)

	cmd := newCleanCmd()
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCleanCmd_WithArgs(t *testing.T) {
	provider := &commandProvider{name: "alpha"}
	setupCommandEngine(t, provider)

	cmd := newCleanCmd()
	cmd.SetArgs([]string{"alpha"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clean command failed: %v", err)
	}
	if !provider.cleanCalled {
		t.Fatal("expected clean to be called")
	}
}

// Round 1: Flag renames and moves

func TestRootCmdNoSSHFlag(t *testing.T) {
	cmd := NewRootCmd("1.0.0")
	if cmd.PersistentFlags().Lookup("ssh-config-path") != nil {
		t.Fatal("ssh-config-path must not be a root persistent flag")
	}
}

func TestGenerateCmdSSHFlag(t *testing.T) {
	cmd := newGenerateCmd()
	if cmd.Flags().Lookup("ssh-config-path") == nil {
		t.Fatal("expected ssh-config-path flag on generate")
	}
}

func TestGenerateCmdEKSFlags(t *testing.T) {
	cmd := newGenerateCmd()
	if cmd.Flags().Lookup("eks-regions") == nil {
		t.Fatal("expected eks-regions flag on generate")
	}
	if cmd.Flags().Lookup("eks-roles") == nil {
		t.Fatal("expected eks-roles flag on generate")
	}
	if cmd.Flags().Lookup("kube-regions") != nil {
		t.Fatal("kube-regions flag must be removed (renamed to eks-regions)")
	}
	if cmd.Flags().Lookup("kube-roles") != nil {
		t.Fatal("kube-roles flag must be removed (renamed to eks-roles)")
	}
}

// Round 2: Viper / env vars

func runListCmd(t *testing.T) error {
	t.Helper()
	cmd := NewRootCmd("test")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
}

func TestEnvVarDryRun(t *testing.T) {
	t.Setenv("CFGCTL_DRY_RUN", "true")
	dryRun = false
	cfgFile = ""
	noBackup = false
	debug = false
	verbose = false

	if err := runListCmd(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !config.DryRun {
		t.Fatal("expected DryRun=true from CFGCTL_DRY_RUN env var")
	}
}

func TestEnvVarNoBackup(t *testing.T) {
	t.Setenv("CFGCTL_NO_BACKUP", "true")
	noBackup = false
	dryRun = false
	cfgFile = ""
	debug = false
	verbose = false

	if err := runListCmd(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !config.NoBackup {
		t.Fatal("expected NoBackup=true from CFGCTL_NO_BACKUP env var")
	}
}

func TestEnvVarDebug(t *testing.T) {
	t.Setenv("CFGCTL_DEBUG", "true")
	debug = false
	dryRun = false
	cfgFile = ""
	noBackup = false
	verbose = false

	if err := runListCmd(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// debug enables debug log level — verify initializeComponents ran without error
	// (logging level isn't directly inspectable; the test validates no error + the var is set)
	if !debug {
		t.Fatal("expected debug=true from CFGCTL_DEBUG env var")
	}
}

func TestEnvVarVerbose(t *testing.T) {
	t.Setenv("CFGCTL_VERBOSE", "true")
	verbose = false
	dryRun = false
	cfgFile = ""
	noBackup = false
	debug = false

	if err := runListCmd(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !config.Verbose {
		t.Fatal("expected Verbose=true from CFGCTL_VERBOSE env var")
	}
}

func TestEnvVarConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cfgctl-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	_ = tmpFile.Close()

	t.Setenv("CFGCTL_CONFIG", tmpFile.Name())
	cfgFile = ""
	dryRun = false
	noBackup = false
	debug = false
	verbose = false

	if err := runListCmd(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgFile != tmpFile.Name() {
		t.Fatalf("expected cfgFile=%q from CFGCTL_CONFIG env var, got %q", tmpFile.Name(), cfgFile)
	}
}

// Round 3: Provider validation

func TestGenerateCmdUnknownProvider(t *testing.T) {
	setupCommandEngine(t, &commandProvider{name: "aws"})

	cmd := newGenerateCmd()
	cmd.SetArgs([]string{"foo"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected 'unknown provider' in error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "aws") {
		t.Fatalf("expected valid provider list in error message, got: %v", err)
	}
}

func TestCleanCmdUnknownProvider(t *testing.T) {
	setupCommandEngine(t, &commandProvider{name: "aws"})

	cmd := newCleanCmd()
	cmd.SetArgs([]string{"foo"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected 'unknown provider' in error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "aws") {
		t.Fatalf("expected valid provider list in error message, got: %v", err)
	}
}

func TestGenerateCmdConflictingFlags(t *testing.T) {
	setupCommandEngine(t, &commandProvider{name: "kubernetes"})

	cmd := newGenerateCmd()
	cmd.SetArgs([]string{"--kube-merge-only", "--eks-regions", "us-east-1", "kubernetes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for conflicting --kube-merge-only and --eks-regions flags")
	}
	if !strings.Contains(err.Error(), "kube-merge-only") || !strings.Contains(err.Error(), "eks-regions") {
		t.Fatalf("expected conflict error mentioning both flags, got: %v", err)
	}
}
