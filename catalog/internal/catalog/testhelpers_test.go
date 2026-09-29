package catalog

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	snapshotV1 = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	snapshotV2 = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	snapshotV3 = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	snapshotV4 = uuid.MustParse("00000000-0000-0000-0000-000000000004")
)

func applyPluginSnapshot(t *testing.T, store *MemoryStore, nodes []NodeClaim, relations []RelationClaim) {
	t.Helper()

	_, _, err := store.ApplyPluginSnapshot("test-plugin", snapshotV1, nodes, relations)
	require.NoError(t, err)
}
