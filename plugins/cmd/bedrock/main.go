// bedrock scans available foundational models and their inference endpoints
// for a specific IAM user fetched from a running AWS Bedrock instance.
//
// # Setup
//
//  1. Create (or reuse) an IAM user with programmatic access, and attach a
//     policy granting listing foundational models in Bedrock and getting
//     metric data from CloudWatch. For the sake of testing, the
//     AdministratorAccess policy can be added for a specific IAM user via the
//     root user, however a least-privilege policy is always recommended.
//  2. Generate an access key for that user (IAM -> Users -> Security
//     credentials -> Create access key, "Local code" use case). Copy the
//     secret immediately or download the .csv file which contains the
//     credentials.
//  3. Provide BEDROCK_AWS_ACCESS_KEY_ID and BEDROCK_AWS_SECRET_ACCESS_KEY to
//     the plugin.
//
// Keep placeholder/empty values in the tracked manifest, and instead create
// the secret out-of-band, e.g.:
//
//	kubectl create secret generic catalog-secrets \
//	  --from-literal=BEDROCK_AWS_ACCESS_KEY_ID=... \
//	  --from-literal=BEDROCK_AWS_SECRET_ACCESS_KEY=...
//
// Then:
//
//  4. Verify Bedrock model access in the configured regions (Bedrock -> Model
//     catalog, matching BEDROCK_REGIONS), and confirm you've invoked at least
//     one model per region. CloudWatch only reports InputTokenCount,
//     OutputTokenCount and Invocations for models that have received
//     traffic. Otherwise the plugin just shows zero usage.
//  5. No AWS_REGION env var is needed in the initContainer, since the region
//     is passed explicitly per call via awsconfig.WithRegion(region).
//
// # Known Issues
//
// Currently, AWS Bedrock model fetches all foundational models and their
// inference endpoints available for a specific IAM user. However, this fetch
// now results in ~140 available inference endpoints if the region is
// selected as us-east-1, and ~40-50 available inference endpoints if the
// selected region is eu-central-1. This fetch mechanism causes a small
// delay on the fetch, but with more regions available for a specific IAM
// user, this mechanism must be optimized.
//
// # Environment Variables
//
//   - AWS_ACCESS_KEY_ID (mandatory) - Access key ID of a specific IAM user
//     instance. This key ID, alongside this IAM user's secret access key, is
//     used for authorizing the user to consume resources that they are
//     permitted to.
//
//   - AWS_SECRET_ACCESS_KEY (mandatory) - Access key secret of a specific IAM
//     user instance. This is used with the access key ID as an authorization
//     mechanism.
//
//   - BEDROCK_REGIONS (optional) - Space-separated list of regions. Defaults
//     to us-east-1.
//
//   - BEDROCK_METRICS_LOOKBACK (optional) - Total period over which AWS
//     CloudWatch is queried for inference endpoint specific metrics.
//     Defaults to 24h.
//
//go:generate bash -c "set -euo pipefail; goreadme -use-stdlib-markdown -title 'bedrock plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	propertyKeyOwnedBy           = "owned_by"
	propertyKeyProvider          = "provider"
	propertyKeyRegion            = "region"
	propertyKeyModelName         = "model_name"
	propertyKeyLifecycleStatus   = "lifecycle_status"
	propertyKeyInputModalities   = "input_modalities"
	propertyKeyOutputModalities  = "output_modalities"
	propertyKeyInputTokensTotal  = "input_tokens_total"
	propertyKeyOutputTokensTotal = "output_tokens_total"
	propertyKeyInvocationsTotal  = "invocations_total"
	propertyKeyEndpointStatus    = "endpoint_status"

	providerNameBedrock = "bedrock"

	endpointStatusHealthy   = "healthy"
	endpointStatusUnhealthy = "unhealthy"

	metricNamespaceBedrock           = "AWS/Bedrock"
	metricNameInputTokenCount        = "InputTokenCount"
	metricNameOutputTokenCount       = "OutputTokenCount"
	metricNameInvocations            = "Invocations"
	metricNameInvocationClientErrors = "InvocationClientErrors"
	metricNameInvocationServerErrors = "InvocationServerErrors"
	metricNameInvocationThrottles    = "InvocationThrottles"
	metricDimensionModelID           = "ModelId"
	metricPeriodSeconds              = 3600

	// maxMetricDataQueriesPerRequest is the CloudWatch GetMetricData limit on
	// MetricDataQueries per call.
	maxMetricDataQueriesPerRequest = 500
)

type config struct {
	PathPrefix string `env:"PATH_PREFIX" default:"bedrock"`
	// Regions is a space-separated list of AWS regions to query, e.g. "us-east-1 eu-central-1".
	Regions         []string      `env:"BEDROCK_REGIONS" default:"us-east-1"`
	MetricsLookback time.Duration `env:"BEDROCK_METRICS_LOOKBACK" default:"24h"`
}

type Plugin struct {
	logger              *log.Logger
	config              config
	newBedrockClient    func(ctx context.Context, region string) (listFoundationModelsFunc, error)
	newCloudWatchClient func(ctx context.Context, region string) (getMetricDataFunc, error)
}

// listFoundationModelsFunc is the subset of *bedrock.Client used by this
// plugin, so tests can substitute a fake implementation.
type listFoundationModelsFunc func(context.Context, *bedrock.ListFoundationModelsInput, ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)

// getMetricDataFunc is the subset of *cloudwatch.Client used by this plugin.
type getMetricDataFunc func(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)

func New(cfg config, logger *log.Logger) *Plugin {
	return &Plugin{
		logger: logger,
		config: cfg,
		newBedrockClient: func(ctx context.Context, region string) (listFoundationModelsFunc, error) {
			awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return nil, fmt.Errorf("loading AWS config for region %q: %w", region, err)
			}
			return bedrock.NewFromConfig(awsCfg).ListFoundationModels, nil
		},
		newCloudWatchClient: func(ctx context.Context, region string) (getMetricDataFunc, error) {
			awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return nil, fmt.Errorf("loading AWS config for region %q: %w", region, err)
			}
			return cloudwatch.NewFromConfig(awsCfg).GetMetricData, nil
		},
	}
}

func main() {
	app := pluginmain.New[config]()

	p := New(app.PluginConfig, app.Logger)

	app.Serve(p)
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	var (
		nodes         []pluginapi.NodeClaim
		relations     []pluginapi.RelationClaim
		collectErrors []error
	)

	for _, region := range p.config.Regions {
		region = strings.TrimSpace(region)
		if region == "" {
			continue
		}

		regionNodes, regionRelations, err := p.collectRegion(ctx, region)
		if err != nil {
			collectErrors = append(collectErrors, fmt.Errorf("collecting Bedrock region %q: %w", region, err))
			continue
		}
		nodes = append(nodes, regionNodes...)
		relations = append(relations, regionRelations...)
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, errors.Join(collectErrors...)
}

func (p *Plugin) collectRegion(ctx context.Context, region string) ([]pluginapi.NodeClaim, []pluginapi.RelationClaim, error) {
	models, err := p.listFoundationModels(ctx, region)
	if err != nil {
		return nil, nil, fmt.Errorf("listing foundation models: %w", err)
	}

	usage, err := p.fetchTokenUsage(ctx, region, models)
	if err != nil {
		if p.logger != nil {
			p.logger.Printf("WARN: fetching Bedrock CloudWatch metrics for region %q failed, continuing without usage: %v", region, err)
		}
		usage = map[string]modelUsage{}
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
	)

	for _, model := range models {
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			continue
		}

		modelNode := pluginapi.NodeClaim{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindModel, Path: p.config.PathPrefix + "/" + modelID},
			Properties: pluginapi.PropertyMap{
				propertyKeyOwnedBy: model.ProviderName,
			},
		}
		nodes = append(nodes, modelNode)

		// Only models with recorded invocations in the lookback window are
		// treated as active inference endpoints; ListFoundationModels returns
		// every model available in the region, not ones actually serving traffic.
		if usage[modelID].Invocations == 0 {
			continue
		}

		// The region is folded into the path's last segment, alongside the
		// model ID, rather than inserted as its own segment: the UI reads the
		// second-to-last path segment as the endpoint's "source", and that
		// must stay "bedrock" (matching the litellm plugin's endpoint paths),
		// not the region.
		endpointNode := pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: p.config.PathPrefix + "/" + modelID + "-" + region},
			Properties: model.properties(region, usage[modelID]),
		}
		nodes = append(nodes, endpointNode)

		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindServesModel,
			From: endpointNode.ID,
			To:   modelNode.ID,
		})
	}

	return nodes, relations, nil
}

type foundationModel struct {
	ModelID      string
	ProviderName string
	// Props holds the properties already converted to their final string
	// form (lifecycle status, modalities, ...), ready to be merged into a
	// node's PropertyMap.
	Props pluginapi.PropertyMap
}

type modelUsage struct {
	InputTokens  float64
	OutputTokens float64
	Invocations  float64
	ClientErrors float64
	ServerErrors float64
	Throttles    float64
}

// status reports whether the model showed any client errors, server errors
// or throttles in the lookback window. This is for active inference endpoints.
func (u modelUsage) endpointStatus() string {
	if u.ClientErrors != 0 || u.ServerErrors != 0 || u.Throttles != 0 {
		return endpointStatusUnhealthy
	}
	return endpointStatusHealthy
}

func (m foundationModel) properties(region string, usage modelUsage) pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{
		propertyKeyProvider: providerNameBedrock,
		propertyKeyRegion:   region,
	}
	for key, value := range m.Props {
		properties[key] = value
	}

	if usage.InputTokens != 0 {
		properties[propertyKeyInputTokensTotal] = strconv.FormatFloat(usage.InputTokens, 'f', 0, 64)
	}
	if usage.OutputTokens != 0 {
		properties[propertyKeyOutputTokensTotal] = strconv.FormatFloat(usage.OutputTokens, 'f', 0, 64)
	}
	if usage.Invocations != 0 {
		properties[propertyKeyInvocationsTotal] = strconv.FormatFloat(usage.Invocations, 'f', 0, 64)
		properties[propertyKeyEndpointStatus] = usage.endpointStatus()
	}

	return properties
}

func (p *Plugin) listFoundationModels(ctx context.Context, region string) ([]foundationModel, error) {
	client, err := p.newBedrockClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("creating Bedrock client for region %q: %w", region, err)
	}

	// TODO next step to do filtering by specifying the properties to fill ListFoundationModelsInput struct.
	out, err := client(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, fmt.Errorf("calling Bedrock ListFoundationModels: %w", err)
	}

	models := make([]foundationModel, 0, len(out.ModelSummaries))
	for _, summary := range out.ModelSummaries {
		props := pluginapi.PropertyMap{}
		for key, value := range map[string]string{
			propertyKeyModelName:        derefString(summary.ModelName),
			propertyKeyLifecycleStatus:  lifecycleStatus(summary.ModelLifecycle),
			propertyKeyInputModalities:  joinModalities(summary.InputModalities),
			propertyKeyOutputModalities: joinModalities(summary.OutputModalities),
		} {
			if value != "" {
				props[key] = value
			}
		}

		models = append(models, foundationModel{
			ModelID:      derefString(summary.ModelId),
			ProviderName: derefString(summary.ProviderName),
			Props:        props,
		})
	}

	return models, nil
}

// fetchTokenUsage queries CloudWatch for the AWS/Bedrock InputTokenCount,
// OutputTokenCount, Invocations, InvocationClientErrors,
// InvocationServerErrors and InvocationThrottles metrics, summed over the
// configured lookback window, so usage and health can be compared across
// regions and models.
func (p *Plugin) fetchTokenUsage(ctx context.Context, region string, models []foundationModel) (map[string]modelUsage, error) {
	if len(models) == 0 {
		return map[string]modelUsage{}, nil
	}

	client, err := p.newCloudWatchClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("creating CloudWatch client for region %q: %w", region, err)
	}

	// TODO: use dynamic time intervals in the UI, not just from BEDROCK_METRICS_LOOKBACK
	lookback := p.config.MetricsLookback
	endTime := time.Now().UTC()
	startTime := endTime.Add(-lookback)

	queries := make([]cwtypes.MetricDataQuery, 0, len(models)*6)
	for i, model := range models {
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			continue
		}
		dimensions := []cwtypes.Dimension{
			{Name: aws.String(metricDimensionModelID), Value: aws.String(modelID)},
		}

		queries = append(queries,
			metricQuery(fmt.Sprintf("in%d", i), metricNameInputTokenCount, dimensions),
			metricQuery(fmt.Sprintf("out%d", i), metricNameOutputTokenCount, dimensions),
			metricQuery(fmt.Sprintf("inv%d", i), metricNameInvocations, dimensions),
			metricQuery(fmt.Sprintf("cerr%d", i), metricNameInvocationClientErrors, dimensions),
			metricQuery(fmt.Sprintf("serr%d", i), metricNameInvocationServerErrors, dimensions),
			metricQuery(fmt.Sprintf("thr%d", i), metricNameInvocationThrottles, dimensions),
		)
	}

	// GetMetricData accepts at most maxMetricDataQueriesPerRequest queries per
	// call, and a single batch may still be split across pages via NextToken,
	// so values are accumulated per query ID across batches and pages.
	usageByQueryID := make(map[string]float64, len(queries))
	for batchStart := 0; batchStart < len(queries); batchStart += maxMetricDataQueriesPerRequest {
		batch := queries[batchStart:min(batchStart+maxMetricDataQueriesPerRequest, len(queries))]

		var nextToken *string
		for {
			out, err := client(ctx, &cloudwatch.GetMetricDataInput{
				StartTime:         &startTime,
				EndTime:           &endTime,
				MetricDataQueries: batch,
				NextToken:         nextToken,
			})
			if err != nil {
				return nil, fmt.Errorf("calling CloudWatch GetMetricData: %w", err)
			}

			for _, result := range out.MetricDataResults {
				usageByQueryID[derefString(result.Id)] += sumValues(result.Values)
			}

			if derefString(out.NextToken) == "" {
				break
			}
			nextToken = out.NextToken
		}
	}

	usage := make(map[string]modelUsage, len(models))
	for i, model := range models {
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			continue
		}
		usage[modelID] = modelUsage{
			InputTokens:  usageByQueryID[fmt.Sprintf("in%d", i)],
			OutputTokens: usageByQueryID[fmt.Sprintf("out%d", i)],
			Invocations:  usageByQueryID[fmt.Sprintf("inv%d", i)],
			ClientErrors: usageByQueryID[fmt.Sprintf("cerr%d", i)],
			ServerErrors: usageByQueryID[fmt.Sprintf("serr%d", i)],
			Throttles:    usageByQueryID[fmt.Sprintf("thr%d", i)],
		}
	}

	return usage, nil
}

func metricQuery(id, metricName string, dimensions []cwtypes.Dimension) cwtypes.MetricDataQuery {
	return cwtypes.MetricDataQuery{
		Id: aws.String(id),
		MetricStat: &cwtypes.MetricStat{
			Metric: &cwtypes.Metric{
				Namespace:  aws.String(metricNamespaceBedrock),
				MetricName: aws.String(metricName),
				Dimensions: dimensions,
			},
			Period: aws.Int32(metricPeriodSeconds),
			Stat:   aws.String("Sum"),
		},
	}
}

func sumValues(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	return total
}

func lifecycleStatus(lifecycle *bedrocktypes.FoundationModelLifecycle) string {
	if lifecycle == nil {
		return ""
	}
	return strings.ToLower(string(lifecycle.Status))
}

// joinModalities returns the modalities as a lowercase, comma-separated list,
// e.g. "text,image".
func joinModalities(modalities []bedrocktypes.ModelModality) string {
	names := make([]string, 0, len(modalities))
	for _, m := range modalities {
		names = append(names, string(m))
	}
	return strings.ToLower(strings.Join(names, ","))
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
