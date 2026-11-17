package main

import (
	"context"
	"fmt"
	"io"
	"log"
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

type listMetricsFunc func(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error)

type getMetricDataFunc func(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)

// fakeCloudWatch implements cloudWatchClient from per-operation closures.
type fakeCloudWatch struct {
	listMetrics   listMetricsFunc
	getMetricData getMetricDataFunc
}

func (f fakeCloudWatch) ListMetrics(ctx context.Context, in *cloudwatch.ListMetricsInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return f.listMetrics(ctx, in, opts...)
}

func (f fakeCloudWatch) GetMetricData(ctx context.Context, in *cloudwatch.GetMetricDataInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return f.getMetricData(ctx, in, opts...)
}

func fakeCloudWatchClient(output *cloudwatch.GetMetricDataOutput, err error) getMetricDataFunc {
	return func(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		if err != nil {
			return output, fmt.Errorf("Error while creating fake CloudWatch client: %w", err)
		}
		return output, nil
	}
}

// fakeListMetrics returns a single page of Invocations series, one per model ID.
func fakeListMetrics(modelIDs ...string) listMetricsFunc {
	return func(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
		out := &cloudwatch.ListMetricsOutput{}
		for _, id := range modelIDs {
			out.Metrics = append(out.Metrics, cwtypes.Metric{
				Namespace:  aws.String("AWS/Bedrock"),
				MetricName: aws.String("Invocations"),
				Dimensions: []cwtypes.Dimension{
					{Name: aws.String("ModelId"), Value: aws.String(id)},
				},
			})
		}
		return out, nil
	}
}

func newTestPlugin(regions []string, bc listFoundationModelsFunc, lm listMetricsFunc, cw getMetricDataFunc, cwErr error) *Plugin {
	return &Plugin{
		logger: log.New(io.Discard, "", 0),
		config: config{
			PathPrefix: "bedrock",
			Regions:    regions,
		},
		newBedrockClient: func(context.Context, string) (listFoundationModelsFunc, error) {
			return bc, nil
		},
		newCloudWatchClient: func(context.Context, string) (cloudWatchClient, error) {
			client := fakeCloudWatch{listMetrics: lm, getMetricData: cw}
			if cwErr != nil {
				return nil, fmt.Errorf("CloudWatch client error: %w", cwErr)
			}
			return client, nil
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
	usageAtIndex0 := &cloudwatch.GetMetricDataOutput{
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
		active  []string
		cw      *cloudwatch.GetMetricDataOutput
		cwErr   error
		want    pluginapi.CollectResponse
	}{
		{
			name:    "model and endpoint nodes carry properties, linked by serves_model",
			regions: []string{"us-east-1"},
			models:  []bedrocktypes.FoundationModelSummary{novaMicro},
			active:  []string{"amazon.nova-micro-v1:0"},
			cw:      usageAtIndex0,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.nova-micro-v1:0",
						},
						Properties: pluginapi.PropertyMap{
							"provider_name": "Amazon",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/us-east-1/amazon.nova-micro-v1:0",
						},
						Properties: pluginapi.PropertyMap{
							"region_name":         "us-east-1",
							"model_id":            "amazon.nova-micro-v1:0",
							"model_name":          "Nova Micro",
							"model_lifecycle":     "active",
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
							Path: "bedrock/us-east-1/amazon.nova-micro-v1:0",
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
			active: []string{"amazon.titan-text-express-v1"},
			cw:     noUsage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"provider_name": "",
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
							"provider_name": "",
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
			active: []string{"amazon.titan-text-express-v1"},
			cw:     usageAtIndex1,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"provider_name": "",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/us-east-1/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"region_name":         "us-east-1",
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
							Path: "bedrock/us-east-1/amazon.titan-text-express-v1",
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
			name:    "model without ListMetrics data keeps its model node but gets no endpoint (active model listed second)",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: aws.String("stale-model")},
				{ModelId: aws.String("active-model")},
			},
			active: []string{"active-model"},
			cw:     usageAtIndex1,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID:         pluginapi.NodeID{Kind: "model", Path: "bedrock/stale-model"},
						Properties: pluginapi.PropertyMap{"provider_name": ""},
					},
					{
						ID:         pluginapi.NodeID{Kind: "model", Path: "bedrock/active-model"},
						Properties: pluginapi.PropertyMap{"provider_name": ""},
					},
					{
						ID: pluginapi.NodeID{Kind: "inference_endpoint", Path: "bedrock/us-east-1/active-model"},
						Properties: pluginapi.PropertyMap{
							"region_name":         "us-east-1",
							"model_id":            "active-model",
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
						From: pluginapi.NodeID{Kind: "inference_endpoint", Path: "bedrock/us-east-1/active-model"},
						To:   pluginapi.NodeID{Kind: "model", Path: "bedrock/active-model"},
					},
				},
			},
		},
		{
			name:    "model without ListMetrics data keeps its model node but gets no endpoint (active model listed first)",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: aws.String("active-model")},
				{ModelId: aws.String("stale-model")},
			},
			active: []string{"active-model"},
			cw:     usageAtIndex0,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID:         pluginapi.NodeID{Kind: "model", Path: "bedrock/active-model"},
						Properties: pluginapi.PropertyMap{"provider_name": ""},
					},
					{
						ID: pluginapi.NodeID{Kind: "inference_endpoint", Path: "bedrock/us-east-1/active-model"},
						Properties: pluginapi.PropertyMap{
							"region_name":         "us-east-1",
							"model_id":            "active-model",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
					{
						ID:         pluginapi.NodeID{Kind: "model", Path: "bedrock/stale-model"},
						Properties: pluginapi.PropertyMap{"provider_name": ""},
					},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: "serves_model",
						From: pluginapi.NodeID{Kind: "inference_endpoint", Path: "bedrock/us-east-1/active-model"},
						To:   pluginapi.NodeID{Kind: "model", Path: "bedrock/active-model"},
					},
				},
			},
		},
		{
			name:    "the same invoked model in multiple regions produces one model and one endpoint per region",
			regions: []string{"us-east-1", "eu-central-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{
					ModelId: aws.String("amazon.titan-text-express-v1"),
				},
			},
			active: []string{"amazon.titan-text-express-v1"},
			cw:     usageAtIndex0,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{
						ID: pluginapi.NodeID{
							Kind: "model",
							Path: "bedrock/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"provider_name": "",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/us-east-1/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"region_name":         "us-east-1",
							"model_id":            "amazon.titan-text-express-v1",
							"input_tokens_total":  "3",
							"output_tokens_total": "5",
							"invocations_total":   "1",
							"status":              "healthy",
						},
					},
					{
						ID: pluginapi.NodeID{
							Kind: "inference_endpoint",
							Path: "bedrock/eu-central-1/amazon.titan-text-express-v1",
						},
						Properties: pluginapi.PropertyMap{
							"region_name":         "eu-central-1",
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
							Path: "bedrock/us-east-1/amazon.titan-text-express-v1",
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
							Path: "bedrock/eu-central-1/amazon.titan-text-express-v1",
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
			p := newTestPlugin(tt.regions, bc, fakeListMetrics(tt.active...), cw, tt.cwErr)

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
		newCloudWatchClient: func(context.Context, string) (cloudWatchClient, error) {
			return fakeCloudWatch{
				listMetrics:   fakeListMetrics(),
				getMetricData: fakeCloudWatchClient(&cloudwatch.GetMetricDataOutput{}, nil),
			}, nil
		},
	}

	got, err := p.Collect(context.Background())
	require.Error(t, err, "the eu-central-1 failure should be surfaced")
	require.Len(t, got.Nodes, 1, "us-east-1 should still be collected despite eu-central-1 failing")
	assert.Equal(t, "bedrock/amazon.titan-text-express-v1", got.Nodes[0].ID.Path)
}

// ListMetrics results can span several pages; models on later pages must
// still be treated as active.
func TestCollect_PaginatesListMetrics(t *testing.T) {
	bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
		ModelSummaries: []bedrocktypes.FoundationModelSummary{
			{ModelId: aws.String("model-a")},
			{ModelId: aws.String("model-b")},
		},
	}, nil)
	lm := func(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
		if in.NextToken == nil {
			return &cloudwatch.ListMetricsOutput{
				Metrics: []cwtypes.Metric{
					{Dimensions: []cwtypes.Dimension{{Name: aws.String("ModelId"), Value: aws.String("model-a")}}},
				},
				NextToken: aws.String("page-2"),
			}, nil
		}
		return &cloudwatch.ListMetricsOutput{
			Metrics: []cwtypes.Metric{
				{Dimensions: []cwtypes.Dimension{{Name: aws.String("ModelId"), Value: aws.String("model-b")}}},
			},
		}, nil
	}
	var queriedIDs []string
	cw := func(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		for _, q := range in.MetricDataQueries {
			queriedIDs = append(queriedIDs, *q.Id)
		}
		return &cloudwatch.GetMetricDataOutput{}, nil
	}
	p := newTestPlugin([]string{"us-east-1"}, bc, lm, cw, nil)

	_, err := p.Collect(t.Context())
	require.NoError(t, err)

	// "model-a" (index 0) comes from page 1 and "model-b" (index 1) from page 2.
	assert.ElementsMatch(t, []string{
		"in0", "out0", "inv0", "cerr0", "serr0", "thr0",
		"in1", "out1", "inv1", "cerr1", "serr1", "thr1",
	}, queriedIDs)
}

// GetMetricData results can span several pages; values for the same query
// must be summed across pages.
func TestCollect_PaginatesGetMetricData(t *testing.T) {
	bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{
		ModelSummaries: []bedrocktypes.FoundationModelSummary{
			{ModelId: aws.String("model-a")},
		},
	}, nil)
	cw := func(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		if in.NextToken == nil {
			return &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{{Id: aws.String("inv0"), Values: []float64{2}}},
				NextToken:         aws.String("page-2"),
			}, nil
		}
		return &cloudwatch.GetMetricDataOutput{
			MetricDataResults: []cwtypes.MetricDataResult{{Id: aws.String("inv0"), Values: []float64{3}}},
		}, nil
	}
	p := newTestPlugin([]string{"us-east-1"}, bc, fakeListMetrics("model-a"), cw, nil)

	got, err := p.Collect(t.Context())
	require.NoError(t, err)

	require.Len(t, got.Nodes, 2)
	assert.Equal(t, "5", got.Nodes[1].Properties["invocations_total"])
}

// GetMetricData accepts at most 500 queries per call and each model needs 6,
// so 84 models (504 queries) is the smallest count that must be split.
func TestCollect_SplitsMetricQueriesIntoBatches(t *testing.T) {
	var (
		models   []bedrocktypes.FoundationModelSummary
		modelIDs []string
	)
	for i := range 84 {
		id := fmt.Sprintf("model-%d", i)
		models = append(models, bedrocktypes.FoundationModelSummary{ModelId: aws.String(id)})
		modelIDs = append(modelIDs, id)
	}
	bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{ModelSummaries: models}, nil)
	var batchSizes []int
	cw := func(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
		batchSizes = append(batchSizes, len(in.MetricDataQueries))
		return &cloudwatch.GetMetricDataOutput{}, nil
	}
	p := newTestPlugin([]string{"us-east-1"}, bc, fakeListMetrics(modelIDs...), cw, nil)

	_, err := p.Collect(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []int{500, 4}, batchSizes)
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
			p := newTestPlugin([]string{"us-east-1"}, bc, fakeListMetrics("amazon.titan-text-express-v1"), cw, nil)

			got, err := p.Collect(context.Background())
			require.NoError(t, err)

			require.Len(t, got.Nodes, 2)
			assert.Equal(t, "unhealthy", got.Nodes[1].Properties["status"])
		})
	}
}

// GetMetricData is billed per requested metric, so models that have no
// Invocations series in ListMetrics must not be queried.
//
// Each query ID is "<metric prefix><model index>", where the index is the
// model's position in the Bedrock list. Six metrics are requested per model:
// input tokens (in), output tokens (out), invocations (inv), client errors
// (cerr), server errors (serr) and throttles (thr).
func TestCollect_OnlyQueriesModelsListedByListMetrics(t *testing.T) {
	tests := []struct {
		name         string
		models       []bedrocktypes.FoundationModelSummary
		active       []string
		wantQueryIDs []string
	}{
		{
			name: "active model listed second",
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: aws.String("stale-model")},
				{ModelId: aws.String("active-model")},
			},
			active:       []string{"active-model"},
			wantQueryIDs: []string{"in1", "out1", "inv1", "cerr1", "serr1", "thr1"},
		},
		{
			name: "active model listed first",
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: aws.String("active-model")},
				{ModelId: aws.String("stale-model")},
			},
			active:       []string{"active-model"},
			wantQueryIDs: []string{"in0", "out0", "inv0", "cerr0", "serr0", "thr0"},
		},
		{
			name: "no active models means no GetMetricData call",
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: aws.String("stale-model")},
			},
			active:       nil,
			wantQueryIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := fakeBedrockClient(&bedrock.ListFoundationModelsOutput{ModelSummaries: tt.models}, nil)

			var queriedIDs []string
			cw := func(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
				for _, q := range in.MetricDataQueries {
					queriedIDs = append(queriedIDs, *q.Id)
				}
				return &cloudwatch.GetMetricDataOutput{}, nil
			}
			p := newTestPlugin([]string{"us-east-1"}, bc, fakeListMetrics(tt.active...), cw, nil)

			_, err := p.Collect(t.Context())
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.wantQueryIDs, queriedIDs)
		})
	}
}
