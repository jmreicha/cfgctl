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

var errProviderConfigNil = errors.New("steampipe provider configuration is nil")

// Provider implements the core.Provider interface for steampipe configuration.
type Provider struct {
	config *Config
}

// NewProvider creates a new steampipe provider instance.
func NewProvider(config *Config) *Provider {
	if config == nil {
		config = DefaultConfig()
	}
	return &Provider{config: config}
}

// Name returns the unique identifier for this provider.
func (p *Provider) Name() string { return ProviderName }

// Validate checks prerequisites for the provider.
func (p *Provider) Validate(_ context.Context) error {
	if p.config == nil {
		return errProviderConfigNil
	}
	if !p.config.Enabled {
		return nil
	}
	return p.config.Validate()
}

// Generate creates the steampipe AWS connection config file.
func (p *Provider) Generate(_ context.Context, opts *core.GenerateOptions) (*core.Result, error) {
	result := &core.Result{
		Provider:     p.Name(),
		FilesCreated: []string{},
		FilesSkipped: []string{},
		Warnings:     []string{},
		Metadata:     make(map[string]interface{}),
	}

	if err := p.applyGenerateOptions(opts); err != nil {
		return nil, err
	}

	if !p.config.Enabled {
		result.Warnings = append(result.Warnings, "steampipe provider is disabled")
		return result, nil
	}

	outputPath := p.config.ConfigPath
	if _, err := os.Stat(outputPath); err == nil && opts != nil && !opts.Force && !opts.DryRun {
		result.FilesSkipped = append(result.FilesSkipped, outputPath)
		result.Warnings = append(result.Warnings, "config file exists, use --force to overwrite")
		return result, nil
	}

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
	case existingContent == "":
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

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	// #nosec G306 -- config files should be user-readable
	if err := os.WriteFile(outputPath, []byte(finalContent), 0o600); err != nil {
		return nil, fmt.Errorf("write steampipe config: %w", err)
	}

	result.FilesCreated = append(result.FilesCreated, outputPath)
	return result, nil
}

// Backup creates a timestamped backup of the existing config file.
func (p *Provider) Backup(_ context.Context) (string, error) {
	if p.config == nil {
		return "", nil
	}
	return core.BackupFile(p.config.ConfigPath)
}

// NeedsBackup reports whether a backup should be created before generation.
func (p *Provider) NeedsBackup(opts *core.GenerateOptions) (bool, error) {
	if p.config == nil {
		return false, nil
	}
	if opts != nil && opts.DryRun {
		return false, nil
	}
	if !p.config.Enabled {
		return false, nil
	}
	// If the file exists but --force was not supplied, generation will be
	// skipped, so there is nothing to back up.
	if opts != nil && !opts.Force {
		if _, err := os.Stat(p.config.ConfigPath); err == nil {
			return false, nil
		}
	}
	return true, nil
}

// Restore recovers configuration from a backup.
func (p *Provider) Restore(_ context.Context, _ string) error {
	return errors.New("restore not yet implemented for steampipe provider")
}

// Clean removes generated configuration files.
func (p *Provider) Clean(_ context.Context) error {
	if p.config == nil {
		return errProviderConfigNil
	}
	if p.config.ConfigPath == "" {
		return nil
	}
	if err := os.Remove(p.config.ConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove steampipe config: %w", err)
	}
	return nil
}

// applyGenerateOptions merges CLI-supplied options into the provider config.
func (p *Provider) applyGenerateOptions(opts *core.GenerateOptions) error {
	if p.config == nil {
		return errProviderConfigNil
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
func parseAWSProfiles(path string) ([]string, string, error) {
	// #nosec G304 -- path is user-configurable
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Sprintf("AWS config not found at %s, skipping steampipe generation", path), nil
		}
		return nil, "", fmt.Errorf("open AWS config: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var profiles []string
	var currentProfile string // empty when not inside a named profile section
	var autoPopulated bool

	flush := func() {
		if currentProfile != "" && autoPopulated {
			profiles = append(profiles, currentProfile)
		}
		currentProfile = ""
		autoPopulated = false
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			header := strings.TrimSpace(line[1 : len(line)-1])
			// AWS config uses `[profile name]` for named profiles; `[default]`
			// is a special case with no "profile " prefix.
			if strings.HasPrefix(strings.ToLower(header), "profile ") {
				currentProfile = strings.TrimSpace(header[len("profile "):])
			} else if strings.EqualFold(header, "default") {
				currentProfile = "default"
			}
			// Skip sso-session and other non-profile sections.
			continue
		}

		if currentProfile != "" {
			if k, v, ok := splitKV(line); ok {
				if strings.EqualFold(k, "sso_auto_populated") && strings.EqualFold(v, "true") {
					autoPopulated = true
				}
			}
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
