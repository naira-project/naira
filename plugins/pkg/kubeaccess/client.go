package kubeaccess

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func newClient(kubeconfig []byte, endpoint, contextName string) (*kubernetes.Clientset, Check) {
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return nil, failed("configuration", "INVALID_CONFIG", "Cannot parse kubeconfig.")
	}
	if contextName == "" {
		contextName = config.CurrentContext
	}
	selected := config.Contexts[contextName]
	if selected == nil || config.Clusters[selected.Cluster] == nil || config.AuthInfos[selected.AuthInfo] == nil {
		return nil, failed("configuration", "INVALID_CONTEXT", "The selected context, cluster or user is missing.")
	}
	cluster, user := config.Clusters[selected.Cluster], config.AuthInfos[selected.AuthInfo]
	server, parseErr := url.Parse(cluster.Server)
	if parseErr != nil || server.Scheme != "https" || server.Hostname() == "" || server.User != nil || server.RawQuery != "" || server.Fragment != "" ||
		strings.TrimRight(cluster.Server, "/") != strings.TrimRight(endpoint, "/") {
		return nil, failed("configuration", "ENDPOINT_MISMATCH", "The registered HTTPS endpoint must match the selected kubeconfig server.")
	}
	if cluster.InsecureSkipTLSVerify || cluster.ProxyURL != "" || cluster.CertificateAuthority != "" ||
		user.Exec != nil || user.AuthProvider != nil || user.TokenFile != "" || user.ClientCertificate != "" || user.ClientKey != "" ||
		user.Impersonate != "" || len(user.ImpersonateGroups) != 0 || len(user.ImpersonateUserExtra) != 0 || user.ImpersonateUID != "" {
		return nil, failed("configuration", "UNSAFE_CONFIG", "Use embedded credentials and verified TLS; exec, auth providers, impersonation, file references and proxies are not supported.")
	}
	hasCertificate := len(user.ClientCertificateData) != 0
	hasKey := len(user.ClientKeyData) != 0
	if user.Username != "" || user.Password != "" || user.Token != "" && (hasCertificate || hasKey) {
		return nil, failed("configuration", "UNSAFE_CONFIG", "Use either an embedded token or a client certificate and key; mixed methods and basic authentication are not supported.")
	}
	if hasCertificate != hasKey {
		return nil, failed("configuration", "INVALID_CONFIG", "Client certificate authentication requires both an embedded certificate and private key.")
	}
	if strings.TrimSpace(user.Token) == "" && !hasCertificate {
		return nil, failed("configuration", "MISSING_CREDENTIAL", "Kubeconfig must contain an embedded token or a client certificate and private key.")
	}
	restConfig, err := clientcmd.NewNonInteractiveClientConfig(*config, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, failed("configuration", "INVALID_CONFIG", "Cannot build the selected Kubernetes connection.")
	}
	restConfig.Timeout = 8 * time.Second
	restConfig.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, failed("configuration", "INVALID_CONFIG", "Cannot initialize the Kubernetes client.")
	}
	return client, Check{
		Name:    "configuration",
		Passed:  true,
		Code:    "OK",
		Message: "Selected kubeconfig and endpoint accepted.",
	}
}
