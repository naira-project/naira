package auth

import (
	"context"
	"encoding/json"
	"fmt"
	openfga "github.com/openfga/go-sdk"
	. "github.com/openfga/go-sdk/client"
)

type Allowed struct {
	Allowed bool
}

var (
	fgaClient  *OpenFgaClient
	fgaModelID string
)

func SetupOpenfgaClient(apiUrl, storeName, modelJSON string) (*OpenFgaClient, string, error) {
	client, err := createClient(apiUrl)
	if err != nil {
		return nil, "", err
	}

	storeId, err := createStore(client, storeName)
	if err != nil {
		return nil, "", err
	}

	if err := client.SetStoreId(storeId); err != nil {
		return nil, "", fmt.Errorf("setting openfga store id: %w", err)
	}


	modelId, err := writeOpenFGAModel(client, modelJSON)
	if err != nil {
		return nil, "", err
	}

	fgaClient = client
	fgaModelID = modelId

	return client, modelId, nil
}

func GetClient() (*OpenFgaClient, error) {
	if fgaClient == nil {
		return nil, fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return fgaClient, nil
}

func GetModelID() (string, error) {
	if fgaClient == nil {
		return "", fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return fgaModelID, nil
}

func WriteTuples(tuples []openfga.TupleKey) error {
	if fgaClient == nil {
		return fmt.Errorf("openfga client not initialized: call Setup first")
	}
	return writeTuples(fgaClient, tuples, fgaModelID)
}

func CheckTuples(fgaClient *OpenFgaClient, user, relation, object, modelId string) (*Allowed, error) {
	options := ClientCheckOptions{
    	AuthorizationModelId: openfga.PtrString(modelId),
	}

	body := ClientCheckRequest{
		User:     user,
		Relation: relation,
		Object:   object,
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
	resp, err := fgaClient.CreateStore(context.Background()).Body(ClientCreateStoreRequest{Name: name}).Execute()
    if err != nil {
        return "", err
    }
	return resp.Id, nil
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