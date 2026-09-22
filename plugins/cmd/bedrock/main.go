// `bedrock` scans available foundational models and their inference endpoints for a specific IAM user fetched from a running AWS Bedrock instance.
//
// ## Setup
//
// 1. Create (or reuse) an IAM user with programmatic access, and attach a policy granting listing foundational models in Bedrock and getting metric data from Cloudwatch (For the sake of testing, `AdministratorAccess` policy can be added for a specific IAM user via root user, however least-privilege policy is always recommended).
// 2. Generate an access key for that user (IAM -> Users -> Security credentials -> Create access key, "Local code" use case). Copy the secret immediately or download the .csv file which consists of the credentials.
// 3. Provide `BEDROCK_AWS_ACCESS_KEY_ID` and `BEDROCK_AWS_SECRET_ACCESS_KEY` to the plugin:
//   - Keep placeholder/empty values in the tracked manifest, and instead create the secret out-of-band, e.g. `kubectl create secret generic catalog-secrets --from-literal=BEDROCK_AWS_ACCESS_KEY_ID=... --from-literal=BEDROCK_AWS_SECRET_ACCESS_KEY=...`.
//
// 4. Verify Bedrock model access in the configured regions (Bedrock -> Model catalog, matching `BEDROCK_REGIONS`), and confirm you've invoked at least one model per region. CloudWatch only reports `InputTokenCount`/`OutputTokenCount`/`Invocations` for models that have received traffic.Otherwise the plugin just shows zero usage.
// 5. No `AWS_REGION` env var is needed in the initContainer, since the region is passed explicitly per call via `awsconfig.WithRegion(region)`.
//
// ## Known Issues
//
// Currently, AWS Bedrock model fetches `all` foundational models and their inference endpoints available for a specific IAM user. However, this fetch now results with ~140 available inference endpoints, if the region is selected as `us-east-1` and ~40-50 available inference endpoints, if the region is selected as `eu-central-1`. This fetch mechanism provides small delay on the fetch, but with more regions available for a specific IAM user, this mechanism must be optimized.
//
// ## Environment Variables
//
//   - `AWS_ACCESS_KEY_ID` (mandatory) - Access key ID of a specific IAM user instance. This key ID, alongside with this IAM user's secret access key, is used for authorizing user to consume resources that they are permitted to.
//
//   - `AWS_SECRET_ACCESS_KEY` (mandatory) - Access key secret of a specific IAM user instance. This is used with access key ID as an authorization mechanism.
//
//   - `BEDROCK_REGIONS` (optional) - Default region is specified as `us-east-1`. Regions must be specified space-separated.
//
//   - `BEDROCK_METRICS_LOOKBACK` (optional) - Total period to which AWS CloudWatch needs to look for collecting inference endpoint specific metrics. Default is `24h`.
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
	propertyKeyOwnedBy          = "owned_by"
	propertyKeyProvider         = "provider"
	propertyKeyRegion           = "region"
	propertyKeyLifecycleStatus  = "lifecycle_status"
	propertyKeyInputModalities  = "input_modalities"
	propertyKeyOutputModalities = "output_modalities"
	propertyKeyInputTokens      = "input_tokens_total"
	propertyKeyOutputTokens     = "output_tokens_total"
	propertyKeyInvocations      = "invocations_total"
	propertyKeyEndpointStatus   = "status"

	providerNameBedrock = "bedrock"

	endpointStatusHealthy   = "healthy"
	endpointStatusUnhealthy = "unhealthy"

	metricNamespaceBedrock = "AWS/Bedrock"
	metricNameInputTokens  = "InputTokenCount"
	metricNameOutputTokens = "OutputTokenCount"
	metricNameInvocations  = "Invocations"
	metricNameClientErrors = "InvocationClientErrors"
	metricNameServerErrors = "InvocationServerErrors"
	metricNameThrottles    = "InvocationThrottles"
	metricDimensionModelID = "ModelId"
	metricLookbackWindow   = 24 * time.Hour
	metricPeriodSeconds    = 3600
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
	ModelID          string
	ModelName        string
	ProviderName     string
	LifecycleStatus  string
	InputModalities  []string
	OutputModalities []string
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
func (u modelUsage) status() string {
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
	for key, value := range map[string]string{
		propertyKeyLifecycleStatus:  strings.ToLower(m.LifecycleStatus),
		propertyKeyInputModalities:  strings.Join(m.InputModalities, ","),
		propertyKeyOutputModalities: strings.Join(m.OutputModalities, ","),
	} {
		if value != "" {
			properties[key] = value
		}
	}

	if usage.InputTokens != 0 {
		properties[propertyKeyInputTokens] = strconv.FormatFloat(usage.InputTokens, 'f', 0, 64)
	}
	if usage.OutputTokens != 0 {
		properties[propertyKeyOutputTokens] = strconv.FormatFloat(usage.OutputTokens, 'f', 0, 64)
	}
	if usage.Invocations != 0 {
		properties[propertyKeyInvocations] = strconv.FormatFloat(usage.Invocations, 'f', 0, 64)
		properties[propertyKeyEndpointStatus] = usage.status()
	}

	return properties
}

func (p *Plugin) listFoundationModels(ctx context.Context, region string) ([]foundationModel, error) {
	client, err := p.newBedrockClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("Error while initializing Bedrock client: %v", err)
	}

	//TODO next step to do filtering by specifying the properties to fill ListFoundationModelsInput struct.
	out, err := client(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, fmt.Errorf("calling Bedrock ListFoundationModels: %w", err)
	}

	models := make([]foundationModel, 0, len(out.ModelSummaries))
	for _, summary := range out.ModelSummaries {
		models = append(models, foundationModel{
			ModelID:          derefString(summary.ModelId),
			ModelName:        derefString(summary.ModelName),
			ProviderName:     derefString(summary.ProviderName),
			LifecycleStatus:  strings.ToLower(string(lifecycleStatus(summary.ModelLifecycle))),
			InputModalities:  modalitiesToStrings(summary.InputModalities),
			OutputModalities: modalitiesToStrings(summary.OutputModalities),
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
		return nil, fmt.Errorf("Error while initializing CloudWatch client: %v", err)
	}

	//TODO: use dynamic time intervals in the UI, not just from BEDROCK_METRICS_LOOKBACK
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
			metricQuery(fmt.Sprintf("in%d", i), metricNameInputTokens, dimensions),
			metricQuery(fmt.Sprintf("out%d", i), metricNameOutputTokens, dimensions),
			metricQuery(fmt.Sprintf("inv%d", i), metricNameInvocations, dimensions),
			metricQuery(fmt.Sprintf("cerr%d", i), metricNameClientErrors, dimensions),
			metricQuery(fmt.Sprintf("serr%d", i), metricNameServerErrors, dimensions),
			metricQuery(fmt.Sprintf("thr%d", i), metricNameThrottles, dimensions),
		)
	}

	out, err := client(ctx, &cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: queries,
	})
	if err != nil {
		return nil, fmt.Errorf("calling CloudWatch GetMetricData: %w", err)
	}

	usageByQueryID := make(map[string]float64, len(out.MetricDataResults))
	for _, result := range out.MetricDataResults {
		usageByQueryID[derefString(result.Id)] = sumValues(result.Values)
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

func lifecycleStatus(lifecycle *bedrocktypes.FoundationModelLifecycle) bedrocktypes.FoundationModelLifecycleStatus {
	if lifecycle == nil {
		return ""
	}
	return lifecycle.Status
}

func modalitiesToStrings(modalities []bedrocktypes.ModelModality) []string {
	result := make([]string, 0, len(modalities))
	for _, m := range modalities {
		result = append(result, strings.ToLower(string(m)))
	}
	return result
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
