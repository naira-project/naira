package clusterregistry

import (
	"errors"
	"fmt"
	"strings"
)

const TestWorkspaceID = "test-workspace"

var (
	ErrInvalidWorkspace  = errors.New("invalid workspace")
	ErrWorkspaceNotFound = errors.New("workspace not found")
)

// Workspace identifies the scope of a cluster by an opaque, stable ID.
// It contains no permissions and does not prove that the workspace exists.
type Workspace struct {
	ID string
}

// Validate checks the ID's structure, not existence or access permissions.
func (w Workspace) Validate() error {
	if w.ID == "" || w.ID != strings.TrimSpace(w.ID) || strings.ContainsAny(w.ID, "\r\n\t") {
		return fmt.Errorf("workspace ID must be nonempty without surrounding whitespace or control separators: %w", ErrInvalidWorkspace)
	}
	return nil
}
