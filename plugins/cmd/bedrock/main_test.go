package main

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

type fakeBedrockClient struct {
	output *bedrock.ListFoundationModelsOutput
	err    error
}

func (f *fakeBedrockClient) ListFoundationModels(context.Context, *bedrock.ListFoundationModelsInput, ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error) {
	return f.output, f.err
}

type fakeCloudWatchClient struct {
	output *cloudwatch.GetMetricDataOutput
	err    error
}

func (f *fakeCloudWatchClient) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return f.output, f.err
}

func strp(s string) *string { return &s }

func newTestPlugin(regions []string, bc bedrockClient, cw cloudWatchClient, cwErr error) *Plugin {
	return &Plugin{
		logger: log.New(io.Discard, "", 0),
		config: config{PathPrefix: "bedrock", Regions: regions},
		newBedrockClient: func(context.Context, string) (bedrockClient, error) {
			return bc, nil
		},
		newCloudWatchClient: func(context.Context, string) (cloudWatchClient, error) {
			return cw, cwErr
		},
	}
}

func TestCollect(t *testing.T) {
	novaMicro := bedrocktypes.FoundationModelSummary{
		ModelId:          strp("amazon.nova-micro-v1:0"),
		ProviderName:     strp("Amazon"),
		ModelLifecycle:   &bedrocktypes.FoundationModelLifecycle{Status: bedrocktypes.FoundationModelLifecycleStatusActive},
		InputModalities:  []bedrocktypes.ModelModality{bedrocktypes.ModelModalityText},
		OutputModalities: []bedrocktypes.ModelModality{bedrocktypes.ModelModalityText},
	}
	usage := &cloudwatch.GetMetricDataOutput{
		MetricDataResults: []cwtypes.MetricDataResult{
			{Id: strp("in0"), Values: []float64{3}},
			{Id: strp("out0"), Values: []float64{5}},
			{Id: strp("inv0"), Values: []float64{1}},
		},
	}
	noUsage := &cloudwatch.GetMetricDataOutput{}
	usageAtIndex1 := &cloudwatch.GetMetricDataOutput{
		MetricDataResults: []cwtypes.MetricDataResult{
			{Id: strp("in1"), Values: []float64{3}},
			{Id: strp("out1"), Values: []float64{5}},
			{Id: strp("inv1"), Values: []float64{1}},
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
						ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.nova-micro-v1:0"},
						Properties: pluginapi.PropertyMap{"owned_by": "Amazon"},
					},
					{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.nova-micro-v1:0-us-east-1"},
						Properties: pluginapi.PropertyMap{
							"provider": "bedrock", "region": "us-east-1", "lifecycle_status": "active",
							"input_modalities": "text", "output_modalities": "text",
							"input_tokens_total": "3", "output_tokens_total": "5", "invocations_total": "1",
							"status": "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{{
					Kind: pluginapi.RelationKindServesModel,
					From: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.nova-micro-v1:0-us-east-1"},
					To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.nova-micro-v1:0"},
				}},
			},
		},
		{
			name:    "no CloudWatch usage means the model node is kept but no endpoint is created",
			regions: []string{"us-east-1"},
			models:  []bedrocktypes.FoundationModelSummary{{ModelId: strp("amazon.titan-text-express-v1")}},
			cw:      noUsage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"}, Properties: pluginapi.PropertyMap{"owned_by": ""}},
				},
			},
		},
		{
			name:    "CloudWatch failure falls back to no usage, so no endpoint is created",
			regions: []string{"us-east-1"},
			models:  []bedrocktypes.FoundationModelSummary{{ModelId: strp("amazon.titan-text-express-v1")}},
			cwErr:   assert.AnError,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"}, Properties: pluginapi.PropertyMap{"owned_by": ""}},
				},
			},
		},
		{
			name:    "model with blank ID is skipped",
			regions: []string{"us-east-1"},
			models: []bedrocktypes.FoundationModelSummary{
				{ModelId: strp("  ")},
				{ModelId: strp("amazon.titan-text-express-v1")},
			},
			cw: usageAtIndex1,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"}, Properties: pluginapi.PropertyMap{"owned_by": ""}},
					{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-us-east-1"},
						Properties: pluginapi.PropertyMap{
							"provider": "bedrock", "region": "us-east-1",
							"input_tokens_total": "3", "output_tokens_total": "5", "invocations_total": "1",
							"status": "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{{
					Kind: pluginapi.RelationKindServesModel,
					From: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-us-east-1"},
					To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"},
				}},
			},
		},
		{
			name:    "the same invoked model in multiple regions produces one endpoint per region",
			regions: []string{"us-east-1", "eu-central-1"},
			models:  []bedrocktypes.FoundationModelSummary{{ModelId: strp("amazon.titan-text-express-v1")}},
			cw:      usage,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"}, Properties: pluginapi.PropertyMap{"owned_by": ""}},
					{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-us-east-1"},
						Properties: pluginapi.PropertyMap{
							"provider": "bedrock", "region": "us-east-1",
							"input_tokens_total": "3", "output_tokens_total": "5", "invocations_total": "1",
							"status": "healthy",
						},
					},
					{ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"}, Properties: pluginapi.PropertyMap{"owned_by": ""}},
					{
						ID: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-eu-central-1"},
						Properties: pluginapi.PropertyMap{
							"provider": "bedrock", "region": "eu-central-1",
							"input_tokens_total": "3", "output_tokens_total": "5", "invocations_total": "1",
							"status": "healthy",
						},
					},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: pluginapi.RelationKindServesModel,
						From: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-us-east-1"},
						To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"},
					},
					{
						Kind: pluginapi.RelationKindServesModel,
						From: pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: "bedrock/amazon.titan-text-express-v1-eu-central-1"},
						To:   pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: "bedrock/amazon.titan-text-express-v1"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := &fakeBedrockClient{output: &bedrock.ListFoundationModelsOutput{ModelSummaries: tt.models}}
			cw := &fakeCloudWatchClient{output: tt.cw}
			p := newTestPlugin(tt.regions, bc, cw, tt.cwErr)

			got, err := p.Collect(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCollect_ListFoundationModelsErrorIsReportedPerRegion needs a distinct
// bedrockClient per region, so it doesn't fit the table above.
func TestCollect_ListFoundationModelsErrorIsReportedPerRegion(t *testing.T) {
	goodModels := &fakeBedrockClient{output: &bedrock.ListFoundationModelsOutput{
		ModelSummaries: []bedrocktypes.FoundationModelSummary{{ModelId: strp("amazon.titan-text-express-v1")}},
	}}
	failing := &fakeBedrockClient{err: assert.AnError}

	p := &Plugin{
		logger: log.New(io.Discard, "", 0),
		config: config{PathPrefix: "bedrock", Regions: []string{"us-east-1", "eu-central-1"}},
		newBedrockClient: func(_ context.Context, region string) (bedrockClient, error) {
			if region == "eu-central-1" {
				return failing, nil
			}
			return goodModels, nil
		},
		newCloudWatchClient: func(context.Context, string) (cloudWatchClient, error) {
			return &fakeCloudWatchClient{output: &cloudwatch.GetMetricDataOutput{}}, nil
		},
	}

	got, err := p.Collect(context.Background())
	require.Error(t, err, "the eu-central-1 failure should be surfaced")
	require.Len(t, got.Nodes, 1, "us-east-1 should still be collected despite eu-central-1 failing")
	assert.Equal(t, "bedrock/amazon.titan-text-express-v1", got.Nodes[0].ID.Path)
}

func TestCollectMarksEndpointUnhealthyOnErrorsOrThrottles(t *testing.T) {
	tests := []struct {
		name string
		cw   *cloudwatch.GetMetricDataOutput
	}{
		{
			name: "client errors",
			cw: &cloudwatch.GetMetricDataOutput{MetricDataResults: []cwtypes.MetricDataResult{
				{Id: strp("inv0"), Values: []float64{5}},
				{Id: strp("cerr0"), Values: []float64{2}},
			}},
		},
		{
			name: "server errors",
			cw: &cloudwatch.GetMetricDataOutput{MetricDataResults: []cwtypes.MetricDataResult{
				{Id: strp("inv0"), Values: []float64{5}},
				{Id: strp("serr0"), Values: []float64{1}},
			}},
		},
		{
			name: "throttles",
			cw: &cloudwatch.GetMetricDataOutput{MetricDataResults: []cwtypes.MetricDataResult{
				{Id: strp("inv0"), Values: []float64{5}},
				{Id: strp("thr0"), Values: []float64{3}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := &fakeBedrockClient{output: &bedrock.ListFoundationModelsOutput{
				ModelSummaries: []bedrocktypes.FoundationModelSummary{{ModelId: strp("amazon.titan-text-express-v1")}},
			}}
			cw := &fakeCloudWatchClient{output: tt.cw}
			p := newTestPlugin([]string{"us-east-1"}, bc, cw, nil)

			got, err := p.Collect(context.Background())
			require.NoError(t, err)

			require.Len(t, got.Nodes, 2)
			assert.Equal(t, endpointStatusUnhealthy, got.Nodes[1].Properties["status"])
		})
	}
}
