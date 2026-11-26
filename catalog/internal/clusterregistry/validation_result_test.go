package clusterregistry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidationResultValidate(t *testing.T) {
	checkedAt := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	base := ValidationResult{
		Status:        ValidationReady,
		CheckedAt:     checkedAt,
		KubeSystemUID: "installation-a",
		Checks:        []AccessCheck{{Name: "connection", Passed: true}},
	}
	for _, tt := range []struct {
		name   string
		change func(*ValidationResult)
	}{
		{
			name:   "pending is not a completed result",
			change: func(r *ValidationResult) { r.Status = ValidationPending },
		},
		{
			name:   "unknown status",
			change: func(r *ValidationResult) { r.Status = "UNKNOWN" },
		},
		{
			name:   "missing timestamp",
			change: func(r *ValidationResult) { r.CheckedAt = time.Time{} },
		},
		{
			name:   "ready requires a UID",
			change: func(r *ValidationResult) { r.KubeSystemUID = "" },
		},
		{
			name:   "ready requires checks",
			change: func(r *ValidationResult) { r.Checks = nil },
		},
		{
			name:   "ready rejects a failed check",
			change: func(r *ValidationResult) { r.Checks = []AccessCheck{{Name: "services", Code: "FORBIDDEN"}} },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := base
			tt.change(&result)
			require.ErrorIs(t, result.validate(), ErrInvalidCluster)
		})
	}

	require.NoError(t, (ValidationResult{Status: ValidationFailed, CheckedAt: checkedAt}).validate())
}
