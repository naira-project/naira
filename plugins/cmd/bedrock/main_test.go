package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

func fakeBedrockClient(output *bedrock.ListFoundationModelsOutput, err error) listFoundationModelsFunc {
	return func(context.Context, *bedrock.ListFoundationModelsInput, ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error) {
		if err != nil {
			return output, fmt.Errorf("Error while creating fake Bedrock client: %w", err)
		}
		return output, nil
	}
}

func fakeCloudWatchClient(output *cloudwatch.GetMetricDataOutput, err error) getMetricDataFunc {
	return func(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		if err != nil {
			return output, fmt.Errorf("Error while creating fake CloudWatch client: %w", err)
		}
		return output, nil
	}
}

func newTestPlugin(regions []string, bc listFoundationModelsFunc, cw getMetricDataFunc, cwErr error) *Plugin {
	return &Plugin{
		logger: log.New(io.Discard, "", 0),
		config: config{
			PathPrefix: "bedrock",
			Regions:    regions,
		},
		newBedrockClient: func(context.Context, string) (listFoundationModelsFunc, error) {
			return bc, nil
		},
		newCloudWatchClient: func(context.Context, string) (getMetricDataFunc, error) {
			if cwErr != nil {
				return cw, fmt.Errorf("CloudWatch client error: %w", cwErr)
			}
			return cw, nil
		},
	}
}

func TestCollect(t *testing.T) {
	novaMicro := bedrocktypes.FoundationModelSummary{
		ModelId:      aws.String("amazon.nova-micro-v1:0"),
		ModelName:    aws.String("Nova Micro"),
		ProviderName: aws.String("Amazon"),
		ModelLifecycle: &bedrocktypes.FoundationModelLifecycle{
			Status: bedrocktypes.FoundationModelLifecycleStatusActive,
		},
		InputModalities:  []bedrocktypes.ModelModality{bedrocktypes.ModelModalityText},
		OutputModalities: []bedrocktypes.ModelModality{bedrocktypes.ModelModalityText},
	}
	usage := &cloudwatch.GetMetricDataOutput{
		MetricDataResults: []cwtypes.MetricDataResult{
			{
				Id:     aws.String("in0"),
				Values: []float64{3},
			},
			{
				Id:     aws.String("out0"),
				Values: []float64{5},
			},
			{
				Id:     aws.String("inv0"),
				Values: []float64{1},
			},
		},
	}
	noUsage := &cloudwatch.GetMetricDataOutput{}
	usageAtIndex1 := &cloudwatch.GetMetricDataOutput{
		MetricDataResults: []cwtypes.MetricDataResult{
			{
				Id:     aws.String("in1"),
				Values: []float64{3},
			},
			{
				Id:     aws.String("out1"),
				Values: []float64{5},
			},
			{
				Id:     aws.String("inv1"),
				Values: []float64{1},
			},
		},
	}

	tests := []struct {
		name    string
		regions []string
		models  []bedrocktypes.FoundationModelSummary
		cw      *cloudwatch.GetMetricDataOutput
		cwErr   error
		want    pluginapi.CollectResponse
	}{
		{
			name:    "model and endpoint nodes carry properties, linked by serves_model",
			regions: []string{"us-east-1"},
			models:  []bedrocktypes.FoundationModelSummary{novaMicro},
			cw:      usage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.nova-micro-v1:0",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "Amazon",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.nova-micro-v1:0-us-east-1",
						},
						Properties: pluginapi.PropertyMap{
							"provider":            "bedrock",
							"region":              "us-east-1",
							"model_id":            "amazon.nova-micro-v1:0",
							"model_name":          "Nova Micro",
							"lifecycle_status":    "active",
							"input_modalities":    "text",
							"output_modalities":   "text",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: "serves_model",
						From: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.nova-micro-v1:0-us-east-1",
						},
						To: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.nova-micro-v1:0",
						},
					},
				},
			},
		},
		{
			name:    "no CloudWatch usage means the model node is kept but no endpoint is created",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{
					ModelId: aws.String("amazon.titan-text-express-v1"),
				},
			},
			cw: noUsage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "",
						},
					},
				},
			},
		},
		{
			name:    "CloudWatch failure falls back to no usage, so no endpoint is created",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{
					ModelId: aws.String("amazon.titan-text-express-v1"),
				},
			},
			cwErr: assert.AnError,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "",
						},
					},
				},
			},
		},
		{
			name:    "model with blank ID is skipped",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{
					ModelId: aws.String("  "),
				},
				{
					ModelId: aws.String("amazon.titan-text-express-v1"),
				},
			},
			cw: usageAtIndex1,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-us-east-1",
						},
						Properties: pluginapi.PropertyMap{
							"provider":            "bedrock",
							"region":              "us-east-1",
							"model_id":            "amazon.titan-text-express-v1",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: "serves_model",
						From: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-us-east-1",
						},
						To: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
					},
				},
			},
		},
		{
			name:    "the same invoked model in multiple regions produces one endpoint per region",
			regions: []string{"us-east-1", "eu-central-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{
					ModelId: aws.String("amazon.titan-text-express-v1"),
				},
			},
			cw: usage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-us-east-1",
						},
						Properties: pluginapi.PropertyMap{
							"provider":            "bedrock",
							"region":              "us-east-1",
							"model_id":            "amazon.titan-text-express-v1",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"owned_by": "",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-eu-central-1",
						},
						Properties: pluginapi.PropertyMap{
							"provider":            "bedrock",
							"region":              "eu-central-1",
							"model_id":            "amazon.titan-text-express-v1",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: "serves_model",
						From: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-us-east-1",
						},
						To: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
					},
					{
						Kind: "serves_model",
						From: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/amazon.titan-text-express-v1-eu-central-1",
						},
						To: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
				ModelSummaries: tt.models,
			}, nil)
			cw := fakeCloudWatchClient(tt.cw, nil)
			p := newTestPlugin(tt.regions, bc, cw, tt.cwErr)

			got, err := p.Collect(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCollect_ListFoundationModelsErrorIsReportedPerRegion needs a distinct
// listFoundationModelsFunc per region, so it doesn't fit the table above.
func TestCollect_ListFoundationModelsErrorIsReportedPerRegion(t *testing.T) {
	goodModels := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
		ModelSummaries: []bedrocktypes.FoundationModelSummary{
			{
				ModelId: aws.String("amazon.titan-text-express-v1"),
			},
		},
	}, nil)
	failing := fakeBedrockClient(nil, assert.AnError)

	p := &Plugin{
		logger: log.New(io.Discard, "", 0),
		config: config{
			PathPrefix: "bedrock",
			Regions:    []string{"us-east-1", "eu-central-1"},
		},
		newBedrockClient: func(_ context.Context, region string) (listFoundationModelsFunc, error) {
			if region == "eu-central-1" {
				return failing, nil
			}
			return goodModels, nil
		},
		newCloudWatchClient: func(context.Context, string) (getMetricDataFunc, error) {
			return fakeCloudWatchClient(&cloudwatch.GetMetricDataOutput{}, nil), nil
		},
	}

	got, err := p.Collect(context.Background())
	require.Error(t, err, "the eu-central-1 failure should be surfaced")
	require.Len(t, got.Nodes, 1, "us-east-1 should still be collected despite eu-central-1 failing")
	assert.Equal(t, "bedrock/amazon.titan-text-express-v1", got.Nodes[0].ID.Path)
}

// TestCollect_BatchesAndPaginatesMetricQueries covers regions with enough
// models to exceed the GetMetricData query limit (e.g. ~140 in us-east-1).
func TestCollect_BatchesAndPaginatesMetricQueries(t *testing.T) {
	const modelCount = 140
	models := make([]bedrocktypes.FoundationModelSummary, modelCount)
	for i := range models {
		models[i] = bedrocktypes.FoundationModelSummary{
			ModelId: aws.String(fmt.Sprintf("model-%d", i)),
		}
	}
	bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
		ModelSummaries: models,
	}, nil)

	lastIndex := modelCount - 1
	var batchSizes []int
	cw := func(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		if len(in.MetricDataQueries) > maxMetricDataQueriesPerRequest {
			return nil, fmt.Errorf("got %d queries, want at most %d", len(in.MetricDataQueries), maxMetricDataQueriesPerRequest)
		}
		if in.NextToken == nil {
			batchSizes = append(batchSizes, len(in.MetricDataQueries))
		}

		var ids []string
		for _, q := range in.MetricDataQueries {
			ids = append(ids, *q.Id)
		}
		if !slices.Contains(ids, fmt.Sprintf("inv%d", lastIndex)) {
			return &cloudwatch.GetMetricDataOutput{}, nil
		}
		// Split the last model's invocations across two pages.
		if in.NextToken == nil {
			return &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:     aws.String(fmt.Sprintf("inv%d", lastIndex)),
						Values: []float64{2},
					},
				},
				NextToken: aws.String("page-2"),
			}, nil
		}
		return &cloudwatch.GetMetricDataOutput{
			MetricDataResults: []cwtypes.MetricDataResult{
				{
					Id:     aws.String(fmt.Sprintf("inv%d", lastIndex)),
					Values: []float64{3},
				},
			},
		}, nil
	}
	p := newTestPlugin([]string{"us-east-1"}, bc, cw, nil)

	got, err := p.Collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []int{500, 340}, batchSizes)
	require.Len(t, got.Nodes, modelCount+1)
	endpoint := got.Nodes[len(got.Nodes)-1]
	assert.Equal(t, "inference_endpoint", endpoint.ID.Kind)
	assert.Equal(t, fmt.Sprintf("bedrock/model-%d-us-east-1", lastIndex), endpoint.ID.Path)
	assert.Equal(t, "5", endpoint.Properties["invocations_total"])
}

func TestCollectMarksEndpointUnhealthyOnErrorsOrThrottles(t *testing.T) {
	tests := []struct {
		name string
		cw   *cloudwatch.GetMetricDataOutput
	}{
		{
			name: "client errors",
			cw: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:     aws.String("inv0"),
						Values: []float64{5},
					},
					{
						Id:     aws.String("cerr0"),
						Values: []float64{2},
					},
				},
			},
		},
		{
			name: "server errors",
			cw: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:     aws.String("inv0"),
						Values: []float64{5},
					},
					{
						Id:     aws.String("serr0"),
						Values: []float64{1},
					},
				},
			},
		},
		{
			name: "throttles",
			cw: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:     aws.String("inv0"),
						Values: []float64{5},
					},
					{
						Id:     aws.String("thr0"),
						Values: []float64{3},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
				ModelSummaries: []bedrocktypes.FoundationModelSummary{
					{
						ModelId: aws.String("amazon.titan-text-express-v1"),
					},
				},
			}, nil)
			cw := fakeCloudWatchClient(tt.cw, nil)
			p := newTestPlugin([]string{"us-east-1"}, bc, cw, nil)

			got, err := p.Collect(context.Background())
			require.NoError(t, err)

			require.Len(t, got.Nodes, 2)
			assert.Equal(t, "unhealthy", got.Nodes[1].Properties["status"])
		})
	}
}
