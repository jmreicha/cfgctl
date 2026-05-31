package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
)

const (
	testProviderAWS        = "aws"
	testProviderKubernetes = "kubernetes"
)

type engineTestProvider struct {
	backupErr   error
	backupPath  string
	backupCheck func(*GenerateOptions) (bool, error)
	cleanErr    error
	generateErr error
	name        string
	result      *Result
	validateErr error

	cleanCalled    bool
	generateCalled bool
	validateCalled bool
	backupCalled   bool

	lastGenerateOpts *GenerateOptions
}

func (p *engineTestProvider) Name() string {
	return p.name
}

func (p *engineTestProvider) Validate(_ context.Context) error {
	p.validateCalled = true
	return p.validateErr
}

func (p *engineTestProvider) Generate(_ context.Context, opts *GenerateOptions) (*Result, error) {
	p.generateCalled = true
	p.lastGenerateOpts = opts
	if p.generateErr != nil {
		return nil, p.generateErr
	}
	if p.result != nil {
		return p.result, nil
	}
	return &Result{Provider: p.name}, nil
}

func (p *engineTestProvider) Backup(_ context.Context) (string, error) {
	p.backupCalled = true
	return p.backupPath, p.backupErr
}

func (p *engineTestProvider) Restore(_ context.Context, _ string) error {
	return nil
}

func (p *engineTestProvider) Clean(_ context.Context) error {
	p.cleanCalled = true
	return p.cleanErr
}

func (p *engineTestProvider) NeedsBackup(opts *GenerateOptions) (bool, error) {
	if p.backupCheck == nil {
		return true, nil
	}
	return p.backupCheck(opts)
}

type testEnabledConfig struct {
	enabled bool
}

func (c testEnabledConfig) IsEnabled() bool {
	return c.enabled
}

func (c testEnabledConfig) Validate() error {
	return nil
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

func TestEngineExecute_AllProviders(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	providerA := &engineTestProvider{name: "a"}
	providerB := &engineTestProvider{name: "b"}

	if err := registry.Register(providerA); err != nil {
		t.Fatalf("failed to register providerA: %v", err)
	}
	if err := registry.Register(providerB); err != nil {
		t.Fatalf("failed to register providerB: %v", err)
	}

	results, err := engine.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["a"] == nil || results["b"] == nil {
		t.Error("expected results for both providers")
	}
}

func TestEngineExecute_MissingToolsSkipsProvider(t *testing.T) {
	findExecutableHook = func(string) (string, bool) {
		return "", false
	}
	t.Cleanup(func() {
		findExecutableHook = nil
	})

	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "aws"}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	results, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if provider.generateCalled {
		t.Error("expected generate to be skipped")
	}
	result := results["aws"]
	if result == nil {
		t.Fatal("expected result for provider")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected warning, got %v", result.Warnings)
	}
	if result.Warnings[0] != "aws provider disabled: missing tools aws" {
		t.Fatalf("expected missing tools warning, got %v", result.Warnings)
	}
}

func TestEngineExecute_MissingToolsHonorsDisabledConfig(t *testing.T) {
	findExecutableHook = func(string) (string, bool) {
		return "", false
	}
	t.Cleanup(func() {
		findExecutableHook = nil
	})

	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	config.SetProviderConfig("aws", testEnabledConfig{enabled: false})
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "aws"}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	results, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if provider.generateCalled {
		t.Error("expected generate to be skipped")
	}
	result := results["aws"]
	if result == nil {
		t.Fatal("expected result for provider")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected warning, got %v", result.Warnings)
	}
	if result.Warnings[0] != "aws provider is disabled" {
		t.Fatalf("expected disabled warning, got %v", result.Warnings)
	}
}

func TestEngineExecute_SpecificProviders(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	providerA := &engineTestProvider{name: "a"}
	providerB := &engineTestProvider{name: "b"}

	if err := registry.Register(providerA); err != nil {
		t.Fatalf("failed to register providerA: %v", err)
	}
	if err := registry.Register(providerB); err != nil {
		t.Fatalf("failed to register providerB: %v", err)
	}

	opts := &ExecuteOptions{Providers: []string{"b"}}
	results, err := engine.Execute(context.Background(), opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results["b"] == nil {
		t.Error("expected result for provider b")
	}
	if providerA.generateCalled {
		t.Error("expected provider a not to be called")
	}
}

func TestEngineExecute_NoProviders(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEngineExecute_ProviderNotFound(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	opts := &ExecuteOptions{Providers: []string{"missing"}}
	_, err := engine.Execute(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEngineExecute_ValidateError(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "bad", validateErr: ErrInvalidProviderName}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !provider.validateCalled {
		t.Error("expected validate to be called")
	}
}

func TestEngineExecute_BackupDeciderSkipsBackup(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{
		name: "aws",
		backupCheck: func(_ *GenerateOptions) (bool, error) {
			return false, nil
		},
	}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{Providers: []string{"aws"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if provider.backupCalled {
		t.Fatal("expected backup to be skipped")
	}
}

func TestEngineExecute_BackupDeciderError(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{
		name: "aws",
		backupCheck: func(_ *GenerateOptions) (bool, error) {
			return false, ErrInvalidProviderName
		},
	}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{Providers: []string{"aws"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if provider.backupCalled {
		t.Fatal("expected backup to be skipped")
	}
}

func TestEngineExecute_GenerateError(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "bad", generateErr: ErrInvalidProviderName}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !provider.generateCalled {
		t.Error("expected generate to be called")
	}
}

func TestEngineValidateAll(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "ok"}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	if err := engine.ValidateAll(context.Background()); err != nil {
		t.Fatalf("ValidateAll failed: %v", err)
	}
	if !provider.validateCalled {
		t.Error("expected validate to be called")
	}
}

func TestEngineValidateAll_Error(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "bad", validateErr: ErrInvalidProviderName}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	if err := engine.ValidateAll(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEngineCleanProvider(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "clean"}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	if err := engine.CleanProvider(context.Background(), "clean"); err != nil {
		t.Fatalf("CleanProvider failed: %v", err)
	}
	if !provider.cleanCalled {
		t.Error("expected clean to be called")
	}
}

func TestEngineCleanProvider_Error(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, newTestLogger())

	provider := &engineTestProvider{name: "clean", cleanErr: ErrInvalidProviderName}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	if err := engine.CleanProvider(context.Background(), "clean"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewEngine_NilLogger(t *testing.T) {
	registry := NewRegistry()
	backupManager := NewBackupManager("")
	config := NewConfig()
	engine := NewEngine(registry, backupManager, config, nil)

	if engine == nil {
		t.Fatal("expected engine, got nil")
	}
	// Should not panic when executing with the default logger
	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error for no providers, got nil")
	}
}

func TestProviderMissingTools_KnownProviders(t *testing.T) {
	findExecutableHook = func(name string) (string, bool) {
		// All tools are "found"
		return "/usr/bin/" + name, true
	}
	t.Cleanup(func() {
		findExecutableHook = nil
	})

	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	tests := []struct {
		provider string
	}{
		{"aws"},
		{"granted"},
		{"kubernetes"},
		{"ssh"},
		{"unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			missing := engine.providerMissingTools(tt.provider)
			if len(missing) != 0 {
				t.Errorf("expected no missing tools for %q when hook returns found, got %v", tt.provider, missing)
			}
		})
	}
}

func TestProviderMissingTools_AllMissing(t *testing.T) {
	findExecutableHook = func(string) (string, bool) {
		return "", false
	}
	t.Cleanup(func() {
		findExecutableHook = nil
	})

	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	tests := []struct {
		provider string
		expected int
	}{
		{"aws", 1},
		{"granted", 1},
		{"kubernetes", 2},
		{"ssh", 1},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			missing := engine.providerMissingTools(tt.provider)
			if len(missing) != tt.expected {
				t.Errorf("expected %d missing tools for %q, got %d: %v", tt.expected, tt.provider, len(missing), missing)
			}
		})
	}
}

func TestReorderProvidersForDependencies_AWSFirst(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderKubernetes},
		&engineTestProvider{name: "steampipe"},
		&engineTestProvider{name: testProviderAWS},
		&engineTestProvider{name: "ssh"},
	}

	result := reorderProvidersForDependencies(providers)

	if len(result) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(result))
	}
	if result[0].Name() != testProviderAWS {
		t.Errorf("expected aws first, got %s", result[0].Name())
	}
	if result[1].Name() != testProviderKubernetes {
		t.Errorf("expected kubernetes second, got %s", result[1].Name())
	}
}

func TestReorderProvidersForDependencies_AWSNotPresent(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderKubernetes},
		&engineTestProvider{name: "steampipe"},
		&engineTestProvider{name: "ssh"},
	}

	result := reorderProvidersForDependencies(providers)

	if len(result) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(result))
	}
	if result[0].Name() != testProviderKubernetes {
		t.Errorf("expected kubernetes first, got %s", result[0].Name())
	}
}

func TestReorderProvidersForDependencies_OnlyAWS(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderAWS},
	}

	result := reorderProvidersForDependencies(providers)

	if len(result) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(result))
	}
	if result[0].Name() != testProviderAWS {
		t.Errorf("expected aws, got %s", result[0].Name())
	}
}

func TestReorderProvidersForDependencies_OnlyOneOther(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderKubernetes},
	}

	result := reorderProvidersForDependencies(providers)

	if len(result) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(result))
	}
	if result[0].Name() != testProviderKubernetes {
		t.Errorf("expected kubernetes, got %s", result[0].Name())
	}
}

func TestReorderProvidersForDependencies_Empty(t *testing.T) {
	providers := []Provider{}

	result := reorderProvidersForDependencies(providers)

	if len(result) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(result))
	}
}

func TestReorderProvidersForDependencies_AWSAtStart(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderAWS},
		&engineTestProvider{name: testProviderKubernetes},
		&engineTestProvider{name: "steampipe"},
	}

	result := reorderProvidersForDependencies(providers)

	if result[0].Name() != testProviderAWS {
		t.Errorf("expected aws first, got %s", result[0].Name())
	}
	if result[1].Name() != testProviderKubernetes {
		t.Errorf("expected kubernetes second, got %s", result[1].Name())
	}
}

func TestReorderProvidersForDependencies_MultipleAWS(t *testing.T) {
	providers := []Provider{
		&engineTestProvider{name: testProviderKubernetes},
		&engineTestProvider{name: testProviderAWS},
		&engineTestProvider{name: "steampipe"},
		&engineTestProvider{name: testProviderAWS},
	}

	result := reorderProvidersForDependencies(providers)

	if result[0].Name() != testProviderAWS {
		t.Errorf("expected aws first, got %s", result[0].Name())
	}
}

// recordingStatus captures all status messages for test assertions.
type recordingStatus struct {
	mu       sync.Mutex
	messages []string
}

func (r *recordingStatus) UpdateStatus(msg string) {
	r.mu.Lock()
	r.messages = append(r.messages, msg)
	r.mu.Unlock()
}

func (r *recordingStatus) contains(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.messages {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func TestSetStatus_Nil(t *testing.T) {
	engine := NewEngine(NewRegistry(), NewBackupManager(""), NewConfig(), newTestLogger())
	engine.SetStatus(nil)

	if _, ok := engine.status.(NoopStatus); !ok {
		t.Error("SetStatus(nil) should keep NoopStatus")
	}
}

func TestSetStatus_Custom(t *testing.T) {
	rec := &recordingStatus{}
	engine := NewEngine(NewRegistry(), NewBackupManager(""), NewConfig(), newTestLogger())
	engine.SetStatus(rec)

	if engine.status != rec {
		t.Error("SetStatus should set the status updater")
	}
}

func TestExecute_SendsStatusMessages(t *testing.T) {
	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	rec := &recordingStatus{}
	engine.SetStatus(rec)

	providerA := &engineTestProvider{name: "alpha"}
	providerB := &engineTestProvider{name: "beta"}
	if err := registry.Register(providerA); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(providerB); err != nil {
		t.Fatal(err)
	}

	_, err := engine.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !rec.contains("Generating alpha configuration...") {
		t.Errorf("expected status message for alpha, got: %v", rec.messages)
	}
	if !rec.contains("Generating beta configuration...") {
		t.Errorf("expected status message for beta, got: %v", rec.messages)
	}
}

func TestExecute_StatusIncludesProviderCount(t *testing.T) {
	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	rec := &recordingStatus{}
	engine.SetStatus(rec)

	if err := registry.Register(&engineTestProvider{name: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&engineTestProvider{name: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&engineTestProvider{name: "c"}); err != nil {
		t.Fatal(err)
	}

	_, err := engine.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !rec.contains("[1/3]") {
		t.Errorf("expected [1/3] in status, got: %v", rec.messages)
	}
	if !rec.contains("[3/3]") {
		t.Errorf("expected [3/3] in status, got: %v", rec.messages)
	}
}

func TestExecute_NoStatusForSkippedProviders(t *testing.T) {
	findExecutableHook = func(string) (string, bool) {
		return "", false
	}
	t.Cleanup(func() { findExecutableHook = nil })

	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	rec := &recordingStatus{}
	engine.SetStatus(rec)

	provider := &engineTestProvider{name: "aws"}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if rec.contains("Generating aws") {
		t.Error("should not send status for skipped provider")
	}
}

func TestExecute_StatusPassedToProviders(t *testing.T) {
	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	rec := &recordingStatus{}
	engine.SetStatus(rec)

	statusProvider := &statusAwareProvider{
		engineTestProvider: engineTestProvider{name: "test"},
	}
	if err := registry.Register(statusProvider); err != nil {
		t.Fatal(err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !statusProvider.receivedStatus {
		t.Error("expected provider to receive StatusUpdater in GenerateOptions")
	}
}

// statusAwareProvider checks that GenerateOptions.Status is set.
type statusAwareProvider struct {
	engineTestProvider
	receivedStatus bool
}

func (p *statusAwareProvider) Generate(_ context.Context, opts *GenerateOptions) (*Result, error) {
	p.generateCalled = true
	p.receivedStatus = opts != nil && opts.Status != nil
	return &Result{Provider: p.name}, nil
}

func TestExecute_GenerateError_FriendlyWrapping(t *testing.T) {
	registry := NewRegistry()
	config := NewConfig()
	engine := NewEngine(registry, NewBackupManager(""), config, newTestLogger())

	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "eks.us-west-2.amazonaws.com",
	}
	provider := &engineTestProvider{name: "a", generateErr: fmt.Errorf("list clusters: %w", dnsErr)}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "check your network/VPN connection") {
		t.Errorf("expected friendly message, got: %v", err)
	}
	// Original error should be preserved in the chain.
	var unwrapped *net.DNSError
	if !errors.As(err, &unwrapped) {
		t.Error("original DNS error should be preserved in error chain")
	}
}

func TestFriendlyError_DNSError(t *testing.T) {
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "eks.us-west-2.amazonaws.com",
	}
	wrapped := fmt.Errorf("something: %w", dnsErr)

	result := friendlyError(wrapped)
	if !strings.Contains(result.Error(), "DNS lookup failed") {
		t.Errorf("expected DNS lookup message, got: %v", result)
	}
	if !strings.Contains(result.Error(), "eks.us-west-2.amazonaws.com") {
		t.Errorf("expected hostname in message, got: %v", result)
	}
	var unwrapped *net.DNSError
	if !errors.As(result, &unwrapped) {
		t.Error("original error should be preserved")
	}
}

func TestFriendlyError_NetOpError(t *testing.T) {
	opErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}
	wrapped := fmt.Errorf("call failed: %w", opErr)

	result := friendlyError(wrapped)
	if !strings.Contains(result.Error(), "network error") {
		t.Errorf("expected network error message, got: %v", result)
	}
	if !strings.Contains(result.Error(), "check your network/VPN connection") {
		t.Errorf("expected VPN hint, got: %v", result)
	}
}

func TestFriendlyError_GenericError(t *testing.T) {
	err := errors.New("something broke")
	result := friendlyError(err)
	if result.Error() != err.Error() {
		t.Errorf("generic errors should be returned as-is, got: %v", result)
	}
}

// checkableProvider implements both Provider and Checker for engine Check tests.
type checkableProvider struct {
	engineTestProvider
	checkResults []CheckResult
	checkErr     error
	checkCalled  bool
}

func (p *checkableProvider) Check(_ context.Context) ([]CheckResult, error) {
	p.checkCalled = true
	return p.checkResults, p.checkErr
}

func TestEngineCheck_SkipsNonCheckers(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	plain := &engineTestProvider{name: "plain"}
	if err := registry.Register(plain); err != nil {
		t.Fatal(err)
	}

	results := engine.Check(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("expected no results for non-Checker provider, got %d", len(results))
	}
}

func TestEngineCheck_RunsCheckers(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	p := &checkableProvider{
		engineTestProvider: engineTestProvider{name: "alpha"},
		checkResults:       []CheckResult{{Target: "t", Status: CheckStatusOK}},
	}
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}

	results := engine.Check(context.Background(), nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 provider result, got %d", len(results))
	}
	if !p.checkCalled {
		t.Error("expected Check to be called")
	}
	if results[0].Provider != "alpha" {
		t.Errorf("expected provider %q, got %q", "alpha", results[0].Provider)
	}
	if len(results[0].Results) != 1 {
		t.Errorf("expected 1 check result, got %d", len(results[0].Results))
	}
}

func TestEngineCheck_ResultsSortedByName(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	for _, name := range []string{"zebra", "apple", "mango"} {
		p := &checkableProvider{engineTestProvider: engineTestProvider{name: name}}
		if err := registry.Register(p); err != nil {
			t.Fatal(err)
		}
	}

	results := engine.Check(context.Background(), nil)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	names := []string{results[0].Provider, results[1].Provider, results[2].Provider}
	if names[0] != "apple" || names[1] != "mango" || names[2] != "zebra" {
		t.Errorf("expected sorted results, got %v", names)
	}
}

func TestEngineCheck_SpecificProviders(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	pa := &checkableProvider{engineTestProvider: engineTestProvider{name: "a"}}
	pb := &checkableProvider{engineTestProvider: engineTestProvider{name: "b"}}
	if err := registry.Register(pa); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(pb); err != nil {
		t.Fatal(err)
	}

	results := engine.Check(context.Background(), &CheckOptions{Providers: []string{"a"}})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Provider != "a" {
		t.Errorf("expected provider a, got %q", results[0].Provider)
	}
	if pb.checkCalled {
		t.Error("expected provider b not to be checked")
	}
}

func TestEngineCheck_CheckerError(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	p := &checkableProvider{
		engineTestProvider: engineTestProvider{name: "broken"},
		checkErr:           errors.New("connection refused"),
	}
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}

	results := engine.Check(context.Background(), nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error to be captured in result")
	}
}

func TestEngineCheck_DefaultTimeout(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	p := &checkableProvider{engineTestProvider: engineTestProvider{name: "p"}}
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}

	// nil opts should not panic and should apply the default timeout.
	results := engine.Check(context.Background(), nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestEngineExecute_ReplacePassedToProvider(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	provider := &engineTestProvider{name: "aws"}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	_, err := engine.Execute(context.Background(), &ExecuteOptions{Replace: true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if provider.lastGenerateOpts == nil {
		t.Fatal("expected Generate to be called with opts")
	}
	if !provider.lastGenerateOpts.Replace {
		t.Error("expected Replace=true to be passed through to provider Generate")
	}
}

func TestEngineCheck_MixedCheckerAndNonChecker(t *testing.T) {
	registry := NewRegistry()
	engine := NewEngine(registry, NewBackupManager(""), NewConfig(), newTestLogger())

	plain := &engineTestProvider{name: "plain"}
	checker := &checkableProvider{
		engineTestProvider: engineTestProvider{name: "checker"},
		checkResults:       []CheckResult{{Target: "t", Status: CheckStatusOK}},
	}
	if err := registry.Register(plain); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(checker); err != nil {
		t.Fatal(err)
	}

	results := engine.Check(context.Background(), nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (only Checker), got %d", len(results))
	}
	if results[0].Provider != "checker" {
		t.Errorf("expected checker result, got %q", results[0].Provider)
	}
}
