package kubernetes

import (
	"encoding/json"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd/api"
)

const cfgctlExtensionKey = "cfgctl.io/metadata"

type cfgctlMetadata struct {
	ManagedBy   string `json:"managed-by"`
	GeneratedAt string `json:"generated-at"`
	Source      string `json:"source"`
}

func newCfgctlExtension(source string) runtime.Object {
	meta := cfgctlMetadata{
		ManagedBy:   "cfgctl",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      source,
	}
	raw, _ := json.Marshal(meta)
	return &runtime.Unknown{Raw: raw, ContentType: "application/json"}
}

func isCfgctlManaged(extensions map[string]runtime.Object) bool {
	if extensions == nil {
		return false
	}
	_, ok := extensions[cfgctlExtensionKey]
	return ok
}

func stampCfgctlExtensions(config *api.Config, source string) {
	if config == nil {
		return
	}

	ext := newCfgctlExtension(source)

	for _, cluster := range config.Clusters {
		if cluster.Extensions == nil {
			cluster.Extensions = make(map[string]runtime.Object)
		}
		cluster.Extensions[cfgctlExtensionKey] = ext
	}

	for _, authInfo := range config.AuthInfos {
		if authInfo.Extensions == nil {
			authInfo.Extensions = make(map[string]runtime.Object)
		}
		authInfo.Extensions[cfgctlExtensionKey] = ext
	}

	for _, ctx := range config.Contexts {
		if ctx.Extensions == nil {
			ctx.Extensions = make(map[string]runtime.Object)
		}
		ctx.Extensions[cfgctlExtensionKey] = ext
	}
}

// preserveUnmanagedEntries copies entries from existing that are not managed by
// cfgctl into target, without overwriting entries already present in target.
func preserveUnmanagedEntries(target, existing *api.Config) {
	if target == nil || existing == nil {
		return
	}

	ensureKubeconfigDefaults(target)
	ensureKubeconfigDefaults(existing)

	for name, cluster := range existing.Clusters {
		if isCfgctlManaged(cluster.Extensions) {
			continue
		}
		if _, exists := target.Clusters[name]; !exists {
			target.Clusters[name] = cluster
		}
	}

	for name, authInfo := range existing.AuthInfos {
		if isCfgctlManaged(authInfo.Extensions) {
			continue
		}
		if _, exists := target.AuthInfos[name]; !exists {
			target.AuthInfos[name] = authInfo
		}
	}

	for name, ctx := range existing.Contexts {
		if isCfgctlManaged(ctx.Extensions) {
			continue
		}
		if _, exists := target.Contexts[name]; !exists {
			target.Contexts[name] = ctx
		}
	}

	if existing.CurrentContext != "" && target.CurrentContext == "" {
		if ctx, ok := existing.Contexts[existing.CurrentContext]; ok {
			if !isCfgctlManaged(ctx.Extensions) {
				target.CurrentContext = existing.CurrentContext
			}
		}
	}
}
