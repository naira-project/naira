package keycloak

import (
	"context"
	"fmt"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	RealmRoleAIEngineer          = "ai-engineer"
	RealmRoleApplicationEngineer = "application-engineer"
)

var realmRoleByNodeKind = map[string]string{
	pluginapi.NodeKindModel:   RealmRoleAIEngineer,
	pluginapi.NodeKindDataset: RealmRoleAIEngineer,
}

func requiredRealmRoleForNodeKind(kind string) string {
	if role, ok := realmRoleByNodeKind[kind]; ok {
		return role
	}
	return RealmRoleApplicationEngineer
}

// AuthorizeNodeRead checks that the authenticated caller's Keycloak realm
// roles grant them access to nodes of the given kind.
func AuthorizeNodeRead(ctx context.Context, kind string) error {
	claims, ok := claimsFromContext(ctx)
	if !ok || claims.UserID == "" {
		return fmt.Errorf("no authenticated user in request context")
	}

	role := requiredRealmRoleForNodeKind(kind)
	for _, realmRole := range claims.RealmRoles {
		if realmRole == role {
			return nil
		}
	}

	return fmt.Errorf("user %q is missing required realm role %q", claims.UserID, role)
}
