package openfga

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	sdk "github.com/openfga/go-sdk"
	openfga "github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
)

//go:embed model.fga.yaml
var modelDSL string

type Authorizer interface {
	AuthorizeNodeRead(ctx context.Context, node catalog.NodeID, ModelType, Relation string) error
}

type ClientWithModelID struct {
	Client  *openfga.OpenFgaClient
	ModelID string
}

// AuthorizeNodeRead checks the existing OpenFGA tuples to determine whether the
// authenticated caller has the given relation on the requested node.
func (c ClientWithModelID) AuthorizeNodeRead(ctx context.Context, node catalog.NodeID, modelType, relation string) error {
	claims, ok := keycloak.ClaimsFromContext(ctx)
	if !ok || claims.UserID == "" {
		return fmt.Errorf("no authenticated user in request context")
	}

	object := fmt.Sprintf("%s:%s/%s", modelType, node.Kind, node.Path)
	roleTuples := roleContextualTuples(claims, "user:"+claims.UserID)
	allowed, err := CheckTuples(c.Client, "user:"+claims.UserID, relation, object, c.ModelID, roleTuples)
	if err != nil {
		return fmt.Errorf("checking openfga tuples: %w", err)
	}

	if !allowed {
		return fmt.Errorf("user %q is not allowed to %s %s", claims.UserID, relation, object)
	}

	return nil
}

func NewClient(apiUrl, storeName string) (*ClientWithModelID, error) {
	jsonStr, err := transformer.TransformDSLToJSON(modelDSL)
	if err != nil {
		return &ClientWithModelID{}, fmt.Errorf("transforming DSL schema to JSON: %w", err)
	}

	client, err := createClient(apiUrl)
	if err != nil {
		return &ClientWithModelID{}, fmt.Errorf("creating a client for OpenFGA: %w", err)
	}

	storeId, err := createStore(client, storeName)
	if err != nil {
		return &ClientWithModelID{}, fmt.Errorf("creating a store with the client: %w", err)
	}

	if err := client.SetStoreId(storeId); err != nil {
		return &ClientWithModelID{}, fmt.Errorf("setting openfga store id: %w", err)
	}

	modelId, err := writeOpenFGAModel(client, jsonStr)
	if err != nil {
		return &ClientWithModelID{}, fmt.Errorf("writing OpenFGA model: %w", err)
	}

	return &ClientWithModelID{
		Client:  client,
		ModelID: modelId,
	}, nil
}

func (c ClientWithModelID) WriteTuples(tuples []sdk.TupleKey) error {
	if c.Client == nil {
		return fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return writeTuples(c.Client, tuples, c.ModelID)
}

func roleContextualTuples(claims keycloak.TokenClaims, user string) []openfga.ClientContextualTupleKey {
	tuples := make([]openfga.ClientContextualTupleKey, 0, len(claims.RealmRoles))
	for _, role := range claims.RealmRoles {
		tuples = append(tuples, openfga.ClientContextualTupleKey{
			User:     user,
			Relation: "assignee",
			Object:   "role:" + role,
		})
	}
	return tuples
}

func CheckTuples(client *openfga.OpenFgaClient, user, relation, object, modelId string, contextualTuples []openfga.ClientContextualTupleKey) (bool, error) {
	options := openfga.ClientCheckOptions{
		AuthorizationModelId: sdk.PtrString(modelId),
	}

	body := openfga.ClientCheckRequest{
		User:             user,
		Relation:         relation,
		Object:           object,
		ContextualTuples: contextualTuples,
	}

	data, err := client.Check(context.Background()).
		Body(body).
		Options(options).
		Execute()

	if err != nil {
		return false, fmt.Errorf("checking the tuple: %w", err)
	}

	return *data.Allowed, nil
}

func createClient(apiUrl string) (*openfga.OpenFgaClient, error) {
	client, err := openfga.NewSdkClient(&openfga.ClientConfiguration{
		ApiUrl: apiUrl,
	})
	if err != nil {
		return nil, fmt.Errorf("creating a new client for OpenFGA: %w", err)
	}
	return client, nil
}

func createStore(client *openfga.OpenFgaClient, name string) (string, error) {
	existing, err := findStoreByName(client, name)
	if err != nil {
		return "", fmt.Errorf("finding store by name: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	resp, err := client.CreateStore(context.Background()).Body(openfga.ClientCreateStoreRequest{Name: name}).Execute()
	if err != nil {
		return "", fmt.Errorf("creating a store in OpenFGA: %w", err)
	}
	return resp.Id, nil
}

func findStoreByName(client *openfga.OpenFgaClient, name string) (string, error) {
	options := openfga.ClientListStoresOptions{
		PageSize: sdk.PtrInt32(10),
		Name:     sdk.PtrString(name),
	}
	stores, err := client.ListStores(context.Background()).Options(options).Execute()
	if err != nil {
		return "", fmt.Errorf("listing openfga stores: %w", err)
	}
	if len(stores.Stores) == 0 {
		return "", nil
	}
	return stores.Stores[0].Id, nil
}

func writeOpenFGAModel(client *openfga.OpenFgaClient, model string) (string, error) {
	var body sdk.WriteAuthorizationModelRequest
	if err := json.Unmarshal([]byte(model), &body); err != nil {
		return "", fmt.Errorf("unmarshalling the model: %w", err)
	}

	existingId, err := findMatchingAuthorizationModel(client, body)
	if err != nil {
		return "", fmt.Errorf("checking for an existing authorization model: %w", err)
	}
	if existingId != "" {
		return existingId, nil
	}

	data, err := client.WriteAuthorizationModel(context.Background()).
		Body(body).
		Execute()

	if err != nil {
		return "", fmt.Errorf("writing the authorization model to the respective store: %w", err)
	}
	return data.AuthorizationModelId, nil
}

// findMatchingAuthorizationModel returns the ID of the store's latest authorization
// model if the store already has a model, so callers can avoid writing a
// another model on every startup.
func findMatchingAuthorizationModel(client *openfga.OpenFgaClient, desired sdk.WriteAuthorizationModelRequest) (string, error) {
	latest, err := client.ReadLatestAuthorizationModel(context.Background()).Execute()
	if err != nil {
		return "", nil
	}
	if latest.AuthorizationModel == nil {
		return "", nil
	}

	return latest.AuthorizationModel.Id, nil
}


func writeTuples(client *openfga.OpenFgaClient, tuples []sdk.TupleKey, modelId string) error {
	options := openfga.ClientWriteOptions{
		AuthorizationModelId: sdk.PtrString(modelId),
		Conflict: openfga.ClientWriteConflictOptions{
			OnDuplicateWrites: openfga.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
		},
	}

	body := openfga.ClientWriteRequest{
		Writes: tuples,
	}

	_, err := client.Write(context.Background()).
		Body(body).
		Options(options).
		Execute()

	if err != nil {
		return fmt.Errorf("writing the tuples to the model: %w", err)
	}

	return nil
}
