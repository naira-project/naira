package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

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

	providerNameBedrock = "bedrock"

	metricNamespaceBedrock = "AWS/Bedrock"
	metricNameInputTokens  = "InputTokenCount"
	metricNameOutputTokens = "OutputTokenCount"
	metricNameInvocations  = "Invocations"
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
	newBedrockClient    func(ctx context.Context, region string) (bedrockClient, error)
	newCloudWatchClient func(ctx context.Context, region string) (cloudWatchClient, error)
}

// bedrockClient is the subset of *bedrock.Client used by this plugin, so tests
// can substitute a fake implementation.
type bedrockClient interface {
	ListFoundationModels(ctx context.Context, params *bedrock.ListFoundationModelsInput, optFns ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)
}

// cloudWatchClient is the subset of *cloudwatch.Client used by this plugin.
type cloudWatchClient interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

func New(cfg config, logger *log.Logger) *Plugin {
	return &Plugin{
		logger: logger,
		config: cfg,
		newBedrockClient: func(ctx context.Context, region string) (bedrockClient, error) {
			awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return nil, fmt.Errorf("loading AWS config for region %q: %w", region, err)
			}
			return bedrock.NewFromConfig(awsCfg), nil
		},
		newCloudWatchClient: func(ctx context.Context, region string) (cloudWatchClient, error) {
			awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return nil, fmt.Errorf("loading AWS config for region %q: %w", region, err)
			}
			return cloudwatch.NewFromConfig(awsCfg), nil
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

		endpointNode := pluginapi.NodeClaim{
			ID:         pluginapi.NodeID{Kind: pluginapi.NodeKindInferenceEndpoint, Path: p.config.PathPrefix + "/" + region + "/" + modelID},
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
}

func (m foundationModel) properties(region string, usage modelUsage) pluginapi.PropertyMap {
	properties := pluginapi.PropertyMap{
		propertyKeyProvider: providerNameBedrock,
		propertyKeyRegion:   region,
	}
	for key, value := range map[string]string{
		propertyKeyLifecycleStatus:  m.LifecycleStatus,
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
	}

	return properties
}

func (p *Plugin) listFoundationModels(ctx context.Context, region string) ([]foundationModel, error) {
	client, err := p.newBedrockClient(ctx, region)
	if err != nil {
		return nil, err
	}

	out, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, fmt.Errorf("calling Bedrock ListFoundationModels: %w", err)
	}

	models := make([]foundationModel, 0, len(out.ModelSummaries))
	for _, summary := range out.ModelSummaries {
		models = append(models, foundationModel{
			ModelID:          derefString(summary.ModelId),
			ModelName:        derefString(summary.ModelName),
			ProviderName:     derefString(summary.ProviderName),
			LifecycleStatus:  string(lifecycleStatus(summary.ModelLifecycle)),
			InputModalities:  modalitiesToStrings(summary.InputModalities),
			OutputModalities: modalitiesToStrings(summary.OutputModalities),
		})
	}

	return models, nil
}

// fetchTokenUsage queries CloudWatch for the AWS/Bedrock InputTokenCount,
// OutputTokenCount and Invocations metrics, summed over the configured
// lookback window, so usage can be compared across regions and models (e.g.
// eu-central-1 vs. us-east-1 token consumption).
func (p *Plugin) fetchTokenUsage(ctx context.Context, region string, models []foundationModel) (map[string]modelUsage, error) {
	if len(models) == 0 {
		return map[string]modelUsage{}, nil
	}

	client, err := p.newCloudWatchClient(ctx, region)
	if err != nil {
		return nil, err
	}

	lookback := p.config.MetricsLookback
	if lookback <= 0 {
		lookback = metricLookbackWindow
	}
	endTime := time.Now().UTC()
	startTime := endTime.Add(-lookback)

	queries := make([]cwtypes.MetricDataQuery, 0, len(models)*3)
	for i, model := range models {
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			continue
		}
		dimensions := []cwtypes.Dimension{{Name: strPtr(metricDimensionModelID), Value: strPtr(modelID)}}

		queries = append(queries,
			metricQuery(fmt.Sprintf("in%d", i), metricNameInputTokens, dimensions),
			metricQuery(fmt.Sprintf("out%d", i), metricNameOutputTokens, dimensions),
			metricQuery(fmt.Sprintf("inv%d", i), metricNameInvocations, dimensions),
		)
	}

	out, err := client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
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
		}
	}

	return usage, nil
}

func metricQuery(id, metricName string, dimensions []cwtypes.Dimension) cwtypes.MetricDataQuery {
	period := int32(metricPeriodSeconds)
	return cwtypes.MetricDataQuery{
		Id: strPtr(id),
		MetricStat: &cwtypes.MetricStat{
			Metric: &cwtypes.Metric{
				Namespace:  strPtr(metricNamespaceBedrock),
				MetricName: strPtr(metricName),
				Dimensions: dimensions,
			},
			Period: &period,
			Stat:   strPtr("Sum"),
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
		result = append(result, string(m))
	}
	return result
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strPtr(s string) *string {
	return &s
}
