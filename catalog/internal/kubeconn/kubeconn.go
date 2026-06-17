package kubeconn

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// RestConfig resolves a *rest.Config from an explicit kubeconfig path, falling
// back to $KUBECONFIG, then ~/.kube/config, then in-cluster config.
func RestConfig(kubeconfig string) (*rest.Config, error) {
	source := "in-cluster settings"
	if kubeconfig == "" {
		if env := os.Getenv("KUBECONFIG"); env != "" {
			kubeconfig = env
			source = "KUBECONFIG env"
		} else if home := homedir.HomeDir(); home != "" {
			candidate := filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(candidate); err == nil {
				kubeconfig = candidate
				source = candidate
			}
			// if file absent, kubeconfig stays "" → BuildConfigFromFlags tries in-cluster
		}
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("building config from %s: %w", source, err)
	}
	return cfg, nil
}
