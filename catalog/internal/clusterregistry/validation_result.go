package clusterregistry

import (
	"fmt"
	"time"
)

type ValidationStatus string

const (
	ValidationPending ValidationStatus = "PENDING"
	ValidationReady   ValidationStatus = "READY"
	ValidationFailed  ValidationStatus = "FAILED"
)

type AccessCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationResult contains safe check results, never credentials or raw upstream errors.
type ValidationResult struct {
	Status        ValidationStatus `json:"status"`
	CheckedAt     time.Time        `json:"checkedAt"`
	KubeSystemUID string           `json:"kubeSystemUid,omitempty"`
	Checks        []AccessCheck    `json:"checks"`
}

// Successful reports whether all checks passed and an installation UID was observed.
func (result ValidationResult) Successful() bool {
	if result.KubeSystemUID == "" || len(result.Checks) == 0 {
		return false
	}
	for _, check := range result.Checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func (result ValidationResult) validate() error {
	if result.Status != ValidationReady && result.Status != ValidationFailed {
		return fmt.Errorf("validation must finish READY or FAILED: %w", ErrInvalidCluster)
	}
	if result.CheckedAt.IsZero() {
		return fmt.Errorf("validation requires a result timestamp: %w", ErrInvalidCluster)
	}
	if result.Status == ValidationReady && !result.Successful() {
		return fmt.Errorf("ready validation requires successful checks and an observed UID: %w", ErrInvalidCluster)
	}
	return nil
}
