// Package steampipe provides steampipe configuration management.
package steampipe

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmreicha/cfgctl/internal/core"
)

// ProviderName is the unique identifier for the steampipe provider.
const ProviderName = "steampipe"

// Provider implements the core.Provider interface for steampipe configuration.
type Provider struct {
	*core.BaseProvider
	config *Config
}

// NewProvider creates a new steampipe provider instance.
func NewProvider(config *Config) *Provider {
	if config == nil {
		config = DefaultConfig()
	}
	return &Provider{
		BaseProvider: core.NewBaseProvider(ProviderName, config),
		config:       config,
	}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string { return ProviderName }

// Generate creates the steampipe AWS connection config file.
func (p *Provider) Generate(_ context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := core.NewResult(p.Name())

	if err := p.applyGenerateOptions(opts); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "steampipe provider is disabled")
		return result, nil
	}

	outputPath := p.config.ConfigPath
	if core.CheckExistingOutput(outputPath, opts, result) {
		return result, nil
	}

	core.UpdateGenerateStatus(opts, "Reading AWS profiles for steampipe connections...")

	// Resolve AWS config path.
	awsConfigPath := p.config.AWSConfigPath
	if awsConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
		awsConfigPath = filepath.Join(home, ".aws", "config")
	}

	// Read AWS profiles.
	profiles, warn, err := parseAWSProfiles(awsConfigPath)
	if err != nil {
		return nil, err
	}
	if warn != "" {
		result.Warnings = append(result.Warnings, warn)
		return result, nil
	}

	if len(profiles) == 0 {
		result.Warnings = append(result.Warnings, "no AWS profiles found in "+awsConfigPath+", skipping steampipe generation")
		return result, nil
	}

	// Filter profiles if configured.
	if len(p.config.Profiles) > 0 {
		profiles = filterProfiles(profiles, p.config.Profiles)
	}

	if len(profiles) == 0 {
		result.Warnings = append(result.Warnings, "no AWS profiles matched the configured filter")
		return result, nil
	}

	// Deduplicate: one connection per AWS account.
	profiles = dedupeByAccount(profiles, p.config.PreferredRoles)

	// Generate new managed blocks.
	generated := make([]spcBlock, 0, len(profiles))
	for _, profile := range profiles {
		connName := connectionNameForProfile(profile, p.config.ConnectionPrefix)
		regions := resolveRegions(profile, p.config.Regions, p.config.ProfileRegions)
		// Use only the account portion of the profile name (before "/") so
		// that the steampipe plugin resolves credentials by account name rather
		// than by the specific SSO role.
		profileName := profile
		if idx := strings.Index(profile, "/"); idx >= 0 {
			profileName = profile[:idx]
		}
		blockContent := generateConnectionBlock(profileName, connName, regions, p.config.IgnoreErrorCodes)
		generated = append(generated, spcBlock{
			content: blockContent,
			name:    connName,
			managed: true,
		})
	}

	// Merge with existing file if it exists.
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

	finalContent := renderBlocks(finalBlocks)

	result.Metadata["connections"] = len(profiles)

	if opts != nil && opts.DryRun {
		result.Warnings = append(result.Warnings, "dry-run mode: no files were actually created")
		result.Metadata["config_path"] = outputPath
		result.Metadata["config_content"] = finalContent
		result.FilesCreated = append(result.FilesCreated, outputPath)
		return result, nil
	}

	if err := core.WriteConfigFile(outputPath, []byte(finalContent), 0o600); err != nil {
		return nil, fmt.Errorf("write steampipe config: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, outputPath)
	return result, nil
}

// applyGenerateOptions merges CLI-supplied options into the provider config.
func (p *Provider) applyGenerateOptions(opts *core.GenerateOptions) error {
	if p.config == nil {
		return errors.New("steampipe provider configuration is nil")
	}
	if opts != nil && opts.Config != nil {
		cfg, ok := opts.Config.(*Config)
		if !ok {
			return errors.New("steampipe config has unexpected type")
		}
		p.config = cfg
	}
	return p.config.Validate()
}

// parseAWSProfiles reads profile names from an AWS config file.
// Returns (profiles, warning, error). A warning is returned instead of an
// error when the file is missing or empty so other providers can still run.
//
// Only profiles that contain `sso_auto_populated = true` are returned; this
// restricts generation to profiles that were written by an SSO login tool
// rather than manually maintained entries.
func parseAWSProfiles(path string) (_ []string, _ string, err error) {
	// #nosec G304 -- path is user-configurable
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "aws config file not found at " + path + ", skipping steampipe generation", nil
		}
		return nil, "", fmt.Errorf("open aws config: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	type profileState struct {
		name          string
		autoPopulated bool
	}

	var profiles []string
	var current *profileState

	flush := func() {
		if current != nil && current.autoPopulated {
			profiles = append(profiles, current.name)
		}
		current = nil
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			inner := line[1 : len(line)-1]
			if strings.HasPrefix(inner, "profile ") {
				current = &profileState{name: strings.TrimPrefix(inner, "profile ")}
			}
			continue
		}

		if current == nil {
			continue
		}

		if k, v, ok := splitKV(line); ok && k == "sso_auto_populated" && v == "true" {
			current.autoPopulated = true
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("read AWS config: %w", err)
	}

	return profiles, "", nil
}

// splitKV parses a "key = value" line from an AWS config file.
func splitKV(line string) (key, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// filterProfiles returns only profiles whose name appears in the allowed set.
func filterProfiles(profiles []string, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	var out []string
	for _, p := range profiles {
		if set[p] {
			out = append(out, p)
		}
	}
	return out
}

// readFileIfExists returns file content or empty string if the file does not exist.
func readFileIfExists(path string) (string, error) {
	// #nosec G304 -- path is from config
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
