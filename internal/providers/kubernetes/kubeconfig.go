package kubernetes

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd/api"
)

var (
	errManualConfigsEmpty    = errors.New("manual configs are empty")
	errManualExecConfigEmpty = errors.New("manual exec config is empty")
	errNamingPatternInvalid  = errors.New("naming pattern contains unknown placeholders")
)

// BuildKubeconfig constructs a kubeconfig from discovered clusters.
func BuildKubeconfig(clusters []DiscoveredCluster, namingPattern string) (*api.Config, error) {
	namingPattern = strings.TrimSpace(namingPattern)
	if namingPattern == "" {
		return nil, errNamingPatternEmpty
	}

	config := &api.Config{
		Kind:           kubeconfigKind,
		APIVersion:     "v1",
		Clusters:       make(map[string]*api.Cluster),
		AuthInfos:      make(map[string]*api.AuthInfo),
		Contexts:       make(map[string]*api.Context),
		Preferences:    api.Preferences{},
		CurrentContext: "",
	}

	for _, cluster := range clusters {
		name, err := renderNamingPattern(namingPattern, cluster)
		if err != nil {
			return nil, err
		}

		config.Clusters[name] = &api.Cluster{
			Server:                   cluster.Endpoint,
			CertificateAuthorityData: cluster.CAData,
		}

		config.AuthInfos[name] = &api.AuthInfo{
			Exec: execConfigForCluster(cluster),
		}

		config.Contexts[name] = &api.Context{
			Cluster:  name,
			AuthInfo: name,
		}
	}

	stampCfgctlExtensions(config, "aws-discovery")

	return config, nil
}

func buildManualKubeconfig(manualConfigs []ManualConfig) (*api.Config, error) {
	if len(manualConfigs) == 0 {
		return nil, errManualConfigsEmpty
	}

	config := newKubeconfig()

	for _, entry := range manualConfigs {
		clusterName, contextName, userName, err := resolveManualEntryNames(entry)
		if err != nil {
			return nil, err
		}

		endpoint := strings.TrimSpace(entry.ClusterEndpoint)
		if endpoint == "" {
			return nil, fmt.Errorf("manual config %q must define cluster endpoint", manualConfigLabel(entry))
		}

		cluster := &api.Cluster{Server: endpoint}
		if caFile := strings.TrimSpace(entry.ClusterCAFile); caFile != "" {
			cluster.CertificateAuthority = caFile
		}
		if caData := strings.TrimSpace(entry.ClusterCAData); caData != "" {
			decoded, err := decodeManualBase64(caData, "cluster_ca_data", entry)
			if err != nil {
				return nil, err
			}
			cluster.CertificateAuthorityData = decoded
		}

		authInfo, err := buildManualAuthInfo(entry)
		if err != nil {
			return nil, err
		}

		context := &api.Context{
			Cluster:  clusterName,
			AuthInfo: userName,
		}
		if namespace := strings.TrimSpace(entry.ContextSettings.Namespace); namespace != "" {
			context.Namespace = namespace
		}

		config.Clusters[clusterName] = cluster
		config.AuthInfos[userName] = authInfo
		config.Contexts[contextName] = context
	}

	stampCfgctlExtensions(config, "manual")

	return config, nil
}

func execConfigForCluster(cluster DiscoveredCluster) *api.ExecConfig {
	if cluster.AuthMode == "aws-vault" {
		return &api.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1",
			Command:    "aws-vault",
			Args: []string{
				"exec",
				cluster.Profile,
				"--",
				"aws",
				"eks",
				"get-token",
				"--cluster-name",
				cluster.Name,
				"--region",
				cluster.Region,
			},
			Env: []api.ExecEnvVar{
				{
					Name:  "AWS_PROFILE",
					Value: accountFromProfile(cluster.Profile),
				},
			},
			InteractiveMode:    api.IfAvailableExecInteractiveMode,
			ProvideClusterInfo: false,
		}
	}

	return &api.ExecConfig{
		APIVersion: "client.authentication.k8s.io/v1",
		Command:    "aws",
		Args: []string{
			"eks",
			"get-token",
			"--cluster-name",
			cluster.Name,
			"--region",
			cluster.Region,
		},
		Env: []api.ExecEnvVar{
			{
				Name:  "AWS_PROFILE",
				Value: cluster.Profile,
			},
		},
	}
}

func buildManualAuthInfo(entry ManualConfig) (*api.AuthInfo, error) {
	authInfo := &api.AuthInfo{
		Token:    strings.TrimSpace(entry.AuthInfo.Token),
		Username: strings.TrimSpace(entry.AuthInfo.Username),
		Password: strings.TrimSpace(entry.AuthInfo.Password),
	}

	if certFile := strings.TrimSpace(entry.AuthInfo.ClientCertificateFile); certFile != "" {
		authInfo.ClientCertificate = certFile
	}
	if keyFile := strings.TrimSpace(entry.AuthInfo.ClientKeyFile); keyFile != "" {
		authInfo.ClientKey = keyFile
	}

	if certData := strings.TrimSpace(entry.AuthInfo.ClientCertificateData); certData != "" {
		decoded, err := decodeManualBase64(certData, "client_certificate_data", entry)
		if err != nil {
			return nil, err
		}
		authInfo.ClientCertificateData = decoded
	}
	if keyData := strings.TrimSpace(entry.AuthInfo.ClientKeyData); keyData != "" {
		decoded, err := decodeManualBase64(keyData, "client_key_data", entry)
		if err != nil {
			return nil, err
		}
		authInfo.ClientKeyData = decoded
	}

	execConfig, err := buildManualExecConfig(entry.AuthInfo.Exec, entry)
	if err != nil && !errors.Is(err, errManualExecConfigEmpty) {
		return nil, err
	}
	if execConfig != nil {
		authInfo.Exec = execConfig
	}

	return authInfo, nil
}

func buildManualExecConfig(exec ManualExecConfig, entry ManualConfig) (*api.ExecConfig, error) {
	apiVersion := strings.TrimSpace(exec.APIVersion)
	command := strings.TrimSpace(exec.Command)

	if apiVersion == "" && command == "" && len(exec.Args) == 0 && len(exec.Env) == 0 {
		return nil, errManualExecConfigEmpty
	}
	if command == "" {
		return nil, fmt.Errorf("manual config %q exec config requires command", manualConfigLabel(entry))
	}
	if apiVersion == "" {
		apiVersion = "client.authentication.k8s.io/v1"
	}

	execConfig := &api.ExecConfig{
		APIVersion: apiVersion,
		Command:    command,
		Args:       exec.Args,
	}

	if len(exec.Env) > 0 {
		execConfig.Env = buildManualExecEnv(exec.Env)
	}

	return execConfig, nil
}

func buildManualExecEnv(env map[string]string) []api.ExecEnvVar {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}

	sort.Strings(keys)

	vars := make([]api.ExecEnvVar, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(env[key])
		if value == "" {
			continue
		}
		vars = append(vars, api.ExecEnvVar{Name: key, Value: value})
	}

	if len(vars) == 0 {
		return nil
	}
	return vars
}

func resolveManualEntryNames(entry ManualConfig) (string, string, string, error) {
	name := strings.TrimSpace(entry.Name)
	clusterName := strings.TrimSpace(entry.ClusterName)
	contextName := strings.TrimSpace(entry.ContextName)
	userName := strings.TrimSpace(entry.UserName)

	if clusterName == "" {
		clusterName = name
	}
	if contextName == "" {
		contextName = name
	}
	if userName == "" {
		userName = name
	}

	if clusterName == "" || contextName == "" || userName == "" {
		return "", "", "", fmt.Errorf("manual config %q must define cluster, context, and user names", manualConfigLabel(entry))
	}

	return clusterName, contextName, userName, nil
}

func decodeManualBase64(value, field string, entry ManualConfig) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("manual config %q %s is not valid base64: %w", manualConfigLabel(entry), field, err)
	}
	return decoded, nil
}

func manualConfigLabel(entry ManualConfig) string {
	if name := strings.TrimSpace(entry.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(entry.ClusterName); name != "" {
		return name
	}
	if name := strings.TrimSpace(entry.ContextName); name != "" {
		return name
	}
	if name := strings.TrimSpace(entry.UserName); name != "" {
		return name
	}
	return "(unnamed)"
}

func renderNamingPattern(pattern string, cluster DiscoveredCluster) (string, error) {
	replacements := map[string]string{
		"{cluster}": cluster.Name,
		"{profile}": cluster.Profile,
		"{region}":  cluster.Region,
	}

	rendered := pattern
	for token, value := range replacements {
		rendered = strings.ReplaceAll(rendered, token, value)
	}

	if strings.Contains(rendered, "{") || strings.Contains(rendered, "}") {
		return "", fmt.Errorf("%w: %s", errNamingPatternInvalid, pattern)
	}

	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", errors.New("naming pattern resolved to empty name")
	}

	return rendered, nil
}

// accountFromProfile extracts the account portion of a profile name.
// For "account/role" profiles it returns "account"; otherwise the full profile.
func accountFromProfile(profile string) string {
	if idx := strings.Index(profile, "/"); idx >= 0 {
		return profile[:idx]
	}
	return profile
}
