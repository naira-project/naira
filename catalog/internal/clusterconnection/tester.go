// Package clusterconnection resolves operator-approved credentials for Kubernetes access checks.
package clusterconnection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/naira-project/naira/plugins/pkg/kubeaccess"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const maxCredentialBytes = 1 << 20

var errCredentialUnavailable = errors.New("credential is unavailable or not approved")

type Config struct {
	// Directory allows kubeconfig-file references only beneath this directory. Empty disables files.
	Directory string
	// Secrets contains exact namespace/name#key entries. Empty disables Kubernetes Secrets.
	Secrets []string
}

// New returns an in-process adapter with explicit credential access limits.
// Secrets use only the backend's in-cluster identity, never the target kubeconfig.
func New(config Config) (clusterregistry.ConnectionTestFunc, error) {
	if config.Directory != "" {
		absolute, err := filepath.Abs(config.Directory)
		if err != nil {
			return nil, fmt.Errorf("resolve credential directory: %w", err)
		}
		config.Directory = absolute
	}
	allowed := make(map[string]struct{}, len(config.Secrets))
	for _, ref := range config.Secrets {
		if _, _, _, ok := secretParts(ref); !ok {
			return nil, fmt.Errorf("invalid allowed Secret reference; expected namespace/name#key")
		}
		allowed[ref] = struct{}{}
	}
	var client kubernetes.Interface
	if len(allowed) != 0 {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster Secret client configuration: %w", err)
		}
		restConfig.Timeout = 8 * time.Second
		client, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("create Secret client: %w", err)
		}
	}
	return newTester(config.Directory, allowed, client), nil
}

func newTester(directory string, allowed map[string]struct{}, client kubernetes.Interface) clusterregistry.ConnectionTestFunc {
	return func(ctx context.Context, cluster clusterregistry.Cluster) clusterregistry.ValidationResult {
		data, err := resolve(ctx, directory, allowed, client, cluster.CredentialRef)
		if err != nil {
			return clusterregistry.ValidationResult{Checks: []clusterregistry.AccessCheck{{
				Name:    "credential",
				Code:    "CREDENTIAL_UNAVAILABLE",
				Message: "Cannot read this credential. Check the operator allowlist, file mount or Secret permissions.",
			}}}
		}
		defer clear(data)
		result := kubeaccess.Validate(ctx, data, cluster.Kubernetes.Connection.APIEndpoint, cluster.Kubernetes.Connection.Context)
		checks := make([]clusterregistry.AccessCheck, 0, len(result.Checks)+1)
		checks = append(checks, clusterregistry.AccessCheck{Name: "credential", Passed: true, Code: "OK", Message: "Approved credential loaded."})
		for _, check := range result.Checks {
			checks = append(checks, clusterregistry.AccessCheck{
				Name:    check.Name,
				Passed:  check.Passed,
				Code:    check.Code,
				Message: check.Message,
			})
		}
		return clusterregistry.ValidationResult{Checks: checks, KubeSystemUID: result.KubeSystemUID}
	}
}

func resolve(ctx context.Context, directory string, allowed map[string]struct{}, client kubernetes.Interface, reference string) ([]byte, error) {
	if path, ok := strings.CutPrefix(reference, "kubeconfig-file:"); ok && directory != "" {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("require absolute file reference: %w", errCredentialUnavailable)
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return nil, fmt.Errorf("resolve file reference: %w", errCredentialUnavailable)
		}
		root, err := os.OpenRoot(directory)
		if err != nil {
			return nil, fmt.Errorf("open credential directory: %w", errCredentialUnavailable)
		}
		defer root.Close()
		file, err := root.Open(relative)
		if err != nil {
			return nil, fmt.Errorf("open approved credential file: %w", errCredentialUnavailable)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxCredentialBytes {
			return nil, fmt.Errorf("check credential file: %w", errCredentialUnavailable)
		}
		data, err := io.ReadAll(io.LimitReader(file, maxCredentialBytes+1))
		if err != nil || len(data) == 0 || len(data) > maxCredentialBytes {
			return nil, fmt.Errorf("read credential file: %w", errCredentialUnavailable)
		}
		return data, nil
	}
	if ref, ok := strings.CutPrefix(reference, "kubernetes-secret:"); ok && client != nil {
		if _, permitted := allowed[ref]; !permitted {
			return nil, fmt.Errorf("check Secret allowlist: %w", errCredentialUnavailable)
		}
		namespace, name, key, valid := secretParts(ref)
		if !valid {
			return nil, fmt.Errorf("parse Secret reference: %w", errCredentialUnavailable)
		}
		secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get approved Secret: %w", errCredentialUnavailable)
		}
		data := secret.Data[key]
		if len(data) == 0 || len(data) > maxCredentialBytes {
			return nil, fmt.Errorf("read Secret key: %w", errCredentialUnavailable)
		}
		return data, nil
	}
	return nil, fmt.Errorf("resolve credential reference: %w", errCredentialUnavailable)
}

func secretParts(ref string) (namespace, name, key string, ok bool) {
	resource, key, found := strings.Cut(ref, "#")
	namespace, name, split := strings.Cut(resource, "/")
	ok = found && split && namespace != "" && name != "" && key != "" &&
		!strings.ContainsAny(name, "/# \t\r\n") && !strings.ContainsAny(namespace, "/# \t\r\n") && !strings.ContainsAny(key, "/# \t\r\n")
	return namespace, name, key, ok
}
