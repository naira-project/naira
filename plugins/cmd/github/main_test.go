package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

func TestImageReferencesOrg(t *testing.T) {
	tests := []struct {
		name  string
		image string
		org   string
		want  bool
	}{
		{
			name:  "matches organization",
			image: "ghcr.io/naira-project/service:latest",
			org:   "naira-project",
			want:  true,
		},
		{
			name:  "matches case insensitively",
			image: "ghcr.io/Naira-Project/service:latest",
			org:   "naira-project",
			want:  true,
		},
		{
			name:  "does not match another organization",
			image: "ghcr.io/other-org/service:latest",
			org:   "naira-project",
		},
		{
			name:  "does not match an empty organization",
			image: "ghcr.io/naira-project/service:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, imageReferencesOrg(tt.image, tt.org))
		})
	}
}

func TestSingleContainerImage(t *testing.T) {
	tests := []struct {
		name      string
		images    []string
		wantImage string
		wantOK    bool
	}{
		{name: "no containers"},
		{name: "multiple containers", images: []string{"a", "b"}},
		{
			name:      "one container",
			images:    []string{"ghcr.io/acme/service:v1"},
			wantImage: "ghcr.io/acme/service:v1",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImage, gotOK := singleContainerImage(Deployment{Images: tt.images})
			assert.Equal(t, tt.wantImage, gotImage)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestCodeownersClaims(t *testing.T) {
	repoNodeID := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}
	handles := []string{"@acme/team", "@alice"}

	nodes, relations := codeownersClaims(repoNodeID, handles)

	assert.ElementsMatch(t, []pluginapi.NodeClaim{
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@acme/team"}},
		{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@alice"}},
	}, nodes)
	assert.ElementsMatch(t, []pluginapi.RelationClaim{
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@acme/team"}},
		{Kind: pluginapi.RelationKindOwnedBy, From: repoNodeID, To: pluginapi.NodeID{Kind: pluginapi.NodeKindOwner, Path: "@alice"}},
	}, relations)
}

func TestCodeownersClaims_Empty(t *testing.T) {
	nodes, relations := codeownersClaims(pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}, nil)

	require.Empty(t, nodes)
	require.Empty(t, relations)
}

func TestRepoCache(t *testing.T) {
	cache := newRepoCache()
	id := pluginapi.NodeID{Kind: pluginapi.NodeKindGitRepository, Path: "github.com/acme/service"}

	assert.False(t, cache.AlreadyCollected(id))
	cache.MarkCollected(id)
	assert.True(t, cache.AlreadyCollected(id))
}
