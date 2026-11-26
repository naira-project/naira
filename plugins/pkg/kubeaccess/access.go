// Package kubeaccess checks Kubernetes connections and read access without collecting resources.
package kubeaccess

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/url"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Check struct {
	Name    string
	Passed  bool
	Code    string
	Message string
}

type Result struct {
	Checks        []Check
	KubeSystemUID string
}

// CheckAccess uses only the supplied kubeconfig and context. Endpoint must match its
// selected server. Authentication requires either an embedded ServiceAccount token
// or an embedded client certificate and private key, never both.
// Exec, external files, proxies, impersonation, insecure TLS and HTTP are rejected.
// Results contain neither credentials nor raw API errors.
func CheckAccess(ctx context.Context, kubeconfig []byte, endpoint, contextName string) Result {
	result := Result{Checks: make([]Check, 0, 6)}
	client, configuration := newClient(kubeconfig, endpoint, contextName)
	result.Checks = append(result.Checks, configuration)
	if !configuration.Passed {
		return result
	}
	_, err := client.Discovery().RESTClient().Get().AbsPath("/version").DoRaw(ctx)
	result.Checks = append(result.Checks, outcome("connection", "Reached the Kubernetes API with verified TLS; resource checks verify access separately.", err))
	if err != nil {
		return result
	}
	namespace, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	check := outcome("fingerprint", "Read the kube-system namespace UID.", err)
	if err == nil {
		result.KubeSystemUID = string(namespace.UID)
		if result.KubeSystemUID == "" {
			check = failed("fingerprint", "MISSING_UID", "The kube-system namespace has no UID.")
		}
	}
	result.Checks = append(result.Checks, check)
	_, err = client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	result.Checks = append(result.Checks, outcome("namespaces", "Can list namespaces cluster-wide.", err))
	_, err = client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{Limit: 1})
	result.Checks = append(result.Checks, outcome("deployments", "Can list Deployments cluster-wide.", err))
	_, err = client.CoreV1().Services("").List(ctx, metav1.ListOptions{Limit: 1})
	result.Checks = append(result.Checks, outcome("services", "Can list Services cluster-wide.", err))
	return result
}

func failed(name, code, message string) Check {
	return Check{
		Name:    name,
		Code:    code,
		Message: message,
	}
}

func outcome(name, success string, err error) Check {
	if err == nil {
		return Check{
			Name:    name,
			Passed:  true,
			Code:    "OK",
			Message: success,
		}
	}
	code, message := "CONNECTION_ERROR", "Could not reach or read the Kubernetes API."
	var certificateError *url.Error
	var unknownAuthority x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	var hostnameError x509.HostnameError
	var networkError net.Error
	switch {
	case apierrors.IsForbidden(err):
		code, message = "FORBIDDEN", "The supplied credential does not have the required read permission."
	case apierrors.IsUnauthorized(err):
		code, message = "UNAUTHORIZED", "The API rejected the credential; it may be expired or invalid."
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkError) && networkError.Timeout():
		code, message = "TIMEOUT", "The access check timed out."
	case errors.Is(err, context.Canceled):
		code, message = "CANCELED", "The access check was canceled."
	case errors.As(err, &unknownAuthority), errors.As(err, &invalidCertificate), errors.As(err, &hostnameError):
		code, message = "TLS_ERROR", "TLS certificate verification failed."
	case apierrors.IsNotFound(err):
		code, message = "NOT_FOUND", "The required Kubernetes API or namespace was not found."
	case errors.As(err, &certificateError):
		code, message = "CONNECTION_ERROR", "Could not establish the Kubernetes API connection."
	}
	return failed(name, code, message)
}
