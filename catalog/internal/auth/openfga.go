package auth

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	openfga "github.com/openfga/go-sdk"
	. "github.com/openfga/go-sdk/client"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/openfga/language/pkg/go/transformer"
)

//go:embed model.fga.yaml
var modelDSL string

type Allowed struct {
	Allowed bool
}

type OpenfgaClient struct {
	FgaClient  *OpenFgaClient
	FgaModelID string
}

// AuthorizeNodeRead checks the existing OpenFGA tuples to determine whether the
// authenticated caller has the given relation on the requested node.
func (cli OpenfgaClient) AuthorizeNodeRead(ctx context.Context, node catalog.NodeID, fgaModelType, fgaRelation string) error {
	claims, ok := ClaimsFromContext(ctx)
	if !ok || claims.Sub == "" {
		return fmt.Errorf("no authenticated user in request context")
	}

	fgaClient, err := cli.GetClient()
	if err != nil {
		return fmt.Errorf("openfga client not configured: %w", err)
	}

	modelID, err := cli.GetModelID()
	if err != nil {
		return fmt.Errorf("getting openfga model id: %w", err)
	}

	object := fmt.Sprintf("%s:%s/%s", fgaModelType, node.Kind, node.Path)
	roleTuples := roleContextualTuples(claims, "user:"+claims.Sub)
	allowed, err := CheckTuples(fgaClient, "user:"+claims.Sub, fgaRelation, object, modelID, roleTuples)
	if err != nil {
		return fmt.Errorf("checking openfga tuples: %w", err)
	}

	if !allowed.Allowed {
		return fmt.Errorf("user %q is not allowed to %s %s", claims.Sub, fgaRelation, object)
	}

	return nil
}

func SetupOpenfgaClient(apiUrl, storeName string) (OpenfgaClient, error) {
	jsonStr, err := transformer.TransformDSLToJSON(modelDSL)
	if err != nil {
		return OpenfgaClient{}, err
	}

	client, err := createClient(apiUrl)
	if err != nil {
		return OpenfgaClient{}, err
	}

	storeId, err := createStore(client, storeName)
	if err != nil {
		return OpenfgaClient{}, err
	}

	if err := client.SetStoreId(storeId); err != nil {
		return OpenfgaClient{}, fmt.Errorf("setting openfga store id: %w", err)
	}

	modelId, err := writeOpenFGAModel(client, jsonStr)
	if err != nil {
		return OpenfgaClient{}, err
	}

	var cli OpenfgaClient

	cli.FgaClient = client
	cli.FgaModelID = modelId

	return cli, nil
}

func (cli OpenfgaClient) GetClient() (*OpenFgaClient, error) {
	if cli.FgaClient == nil {
		return nil, fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return cli.FgaClient, nil
}

func (cli OpenfgaClient) GetModelID() (string, error) {
	if cli.FgaClient == nil {
		return "", fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return cli.FgaModelID, nil
}

func (cli OpenfgaClient) WriteTuples(tuples []openfga.TupleKey) error {
	if cli.FgaClient == nil {
		return fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return writeTuples(cli.FgaClient, tuples, cli.FgaModelID)
}

func roleContextualTuples(claims TokenClaims, user string) []ClientContextualTupleKey {
	tuples := make([]ClientContextualTupleKey, 0, len(claims.RealmRoles))
	for _, role := range claims.RealmRoles {
		tuples = append(tuples, ClientContextualTupleKey{
			User:     user,
			Relation: "assignee",
			Object:   "role:" + role,
		})
	}
	return tuples
}

func CheckTuples(fgaClient *OpenFgaClient, user, relation, object, modelId string, contextualTuples []ClientContextualTupleKey) (*Allowed, error) {
	options := ClientCheckOptions{
    	AuthorizationModelId: openfga.PtrString(modelId),
	}

	body := ClientCheckRequest{
		User:             user,
		Relation:         relation,
		Object:           object,
		ContextualTuples: contextualTuples,
	}

	data, err := fgaClient.Check(context.Background()).
		Body(body).
		Options(options).
    	Execute()
	
	if err != nil {
		return nil, err
	}

	return &Allowed{Allowed: *data.Allowed}, nil
}

func createClient(apiUrl string) (*OpenFgaClient, error) {
	fgaClient, err := NewSdkClient(&ClientConfiguration{
        ApiUrl: apiUrl,
    })
	if err != nil {
		return nil, err 
	}
	return fgaClient, nil
}

func createStore(fgaClient *OpenFgaClient, name string) (string, error) {
	existing, err := findStoreByName(fgaClient, name)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	resp, err := fgaClient.CreateStore(context.Background()).Body(ClientCreateStoreRequest{Name: name}).Execute()
    if err != nil {
        return "", err
    }
	return resp.Id, nil
}

func findStoreByName(fgaClient *OpenFgaClient, name string) (string, error) {
	options := ClientListStoresOptions{
		PageSize: openfga.PtrInt32(10),
		Name:     openfga.PtrString(name),
	}
	stores, err := fgaClient.ListStores(context.Background()).Options(options).Execute()
	if err != nil {
		return "", fmt.Errorf("listing openfga stores: %w", err)
	}
	if len(stores.Stores) == 0 {
		return "", nil
	}
	return stores.Stores[0].Id, nil
}

func writeOpenFGAModel(fgaClient *OpenFgaClient, model string) (string, error) {
	var body openfga.WriteAuthorizationModelRequest
	if err := json.Unmarshal([]byte(model), &body); err != nil {
		return "", err
	}

	data, err := fgaClient.WriteAuthorizationModel(context.Background()).
		Body(body).
		Execute()

	if err != nil {
		return "", err
	}
	return data.AuthorizationModelId, nil
}

func writeTuples(fgaClient *OpenFgaClient, tuples []openfga.TupleKey, modelId string) error {
	options := ClientWriteOptions{
    	AuthorizationModelId: openfga.PtrString(modelId),
		Conflict: ClientWriteConflictOptions{
        	OnDuplicateWrites: CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
		},
	}

	body := ClientWriteRequest{
    	Writes: tuples,
	}

	_, err := fgaClient.Write(context.Background()).
		Body(body).
		Options(options).
		Execute()

	if err != nil {
		return err
	}

	return nil
}