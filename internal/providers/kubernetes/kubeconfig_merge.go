package kubernetes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const kubeconfigKind = "Config"

var (
	errKubeconfigMissing     = errors.New("kubeconfig not found")
	errKubeconfigPathEmpty   = errors.New("kubeconfig path is empty")
	errMergeSourceUnreadable = errors.New("merge source directory cannot be read")
)

// MergeKubeconfigs merges existing kubeconfigs with discovered clusters.
// Entries in the existing output file that were previously created by cfgctl
// are dropped (they will be regenerated from discovery). Entries without the
// cfgctl extension marker are treated as user-managed and preserved.
func MergeKubeconfigs(outputPath string, mergeConfig MergeConfig, discovered *api.Config) (*api.Config, []string, error) {
	merged := newKubeconfig()

	if outputPath != "" {
		existing, err := loadKubeconfigIfExists(outputPath)
		if err != nil && !errors.Is(err, errKubeconfigMissing) {
			return nil, nil, err
		}
		preserveUnmanagedEntries(merged, existing)
	}

	mergeFiles, err := resolveMergeFiles(mergeConfig)
	if err != nil {
		return nil, nil, err
	}

	for _, path := range mergeFiles {
		config, err := loadKubeconfigIfExists(path)
		if err != nil {
			return nil, nil, err
		}
		mergeInto(merged, config)
	}

	if discovered != nil {
		mergeInto(merged, discovered)
	}

	ensureKubeconfigDefaults(merged)
	return merged, mergeFiles, nil
}

func resolveMergeFiles(mergeConfig MergeConfig) ([]string, error) {
	sourceDir := strings.TrimSpace(mergeConfig.SourceDir)
	if sourceDir == "" {
		return nil, errMergeSourceDirEmpty
	}

	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat merge source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("merge source path is not a directory: %s", sourceDir)
	}

	includePatterns := normalizePatterns(mergeConfig.IncludePatterns)
	excludePatterns := normalizePatterns(mergeConfig.ExcludePatterns)

	matches := make(map[string]struct{})
	walkErr := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		base := filepath.Base(path)

		if !matchesPattern(base, rel, includePatterns) {
			return nil
		}
		if matchesPattern(base, rel, excludePatterns) {
			return nil
		}

		matches[path] = struct{}{}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("%w: %w", errMergeSourceUnreadable, walkErr)
	}

	files := make([]string, 0, len(matches))
	for file := range matches {
		files = append(files, file)
	}
	sort.Strings(files)

	return files, nil
}

func loadKubeconfigIfExists(path string) (*api.Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errKubeconfigPathEmpty
	}

	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errKubeconfigMissing
		}
		return nil, fmt.Errorf("load kubeconfig %s: %w", path, err)
	}

	ensureKubeconfigDefaults(config)
	return config, nil
}

func newKubeconfig() *api.Config {
	config := api.NewConfig()
	config.Kind = kubeconfigKind
	config.APIVersion = "v1"
	config.Preferences = api.Preferences{}
	config.CurrentContext = ""
	return config
}

func ensureKubeconfigDefaults(config *api.Config) {
	if config == nil {
		return
	}

	if config.Kind == "" {
		config.Kind = kubeconfigKind
	}
	if config.APIVersion == "" {
		config.APIVersion = "v1"
	}
	if config.Clusters == nil {
		config.Clusters = make(map[string]*api.Cluster)
	}
	if config.AuthInfos == nil {
		config.AuthInfos = make(map[string]*api.AuthInfo)
	}
	if config.Contexts == nil {
		config.Contexts = make(map[string]*api.Context)
	}
	if config.Extensions == nil {
		config.Extensions = make(map[string]runtime.Object)
	}
}

func mergeInto(target, source *api.Config) {
	if target == nil || source == nil {
		return
	}

	ensureKubeconfigDefaults(target)
	ensureKubeconfigDefaults(source)

	if source.Kind != "" {
		target.Kind = source.Kind
	}
	if source.APIVersion != "" {
		target.APIVersion = source.APIVersion
	}
	if source.CurrentContext != "" {
		target.CurrentContext = source.CurrentContext
	}
	if !preferencesEmpty(source.Preferences) {
		target.Preferences = source.Preferences
	}
	if len(source.Extensions) > 0 {
		if target.Extensions == nil {
			target.Extensions = make(map[string]runtime.Object)
		}
		for key, value := range source.Extensions {
			target.Extensions[key] = value
		}
	}

	for key, value := range source.Clusters {
		target.Clusters[key] = value
	}
	for key, value := range source.AuthInfos {
		target.AuthInfos[key] = value
	}
	for key, value := range source.Contexts {
		target.Contexts[key] = value
	}
}

func preferencesEmpty(preferences api.Preferences) bool {
	if preferences.Colors {
		return false
	}
	return len(preferences.Extensions) == 0
}

func matchesPattern(base, rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if matchPattern(pattern, base) || matchPattern(pattern, rel) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, target string) bool {
	if pattern == "" || target == "" {
		return false
	}

	matched, err := filepath.Match(pattern, target)
	if err != nil {
		return false
	}
	return matched
}

func normalizePatterns(patterns []string) []string {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	sort.Strings(normalized)
	return normalized
}
