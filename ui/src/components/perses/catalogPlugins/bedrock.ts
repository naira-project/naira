import type { PanelConfig } from '../types';

// YACE exports each CloudWatch datapoint as a gauge holding the statistic over
// its 1m period (see deploy/dev/stacks/llm-inference/infra/helm/yace-values.yaml),
// so these are plotted as-is rather than rate()d.
// A model node has no region, so without one every region is drawn as its own line.
export function bedrockPanels(modelId: string, region?: string): PanelConfig[] {
  const regionFilter = region ? `, region="${region}"` : '';
  const selector = `{dimension_ModelId="${modelId}"${regionFilter}}`;
  return [
    {
      title: 'Invocations (per 1m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_invocations_sum${selector})`,
    },
    {
      title: 'Input Tokens (per 1m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_input_token_count_sum${selector})`,
    },
    {
      title: 'Output Tokens (per 1m)',
      query: `sum by (dimension_ModelId, region) (aws_bedrock_output_token_count_sum${selector})`,
    },
    {
      title: 'Average Invocation Latency (ms)',
      query: `avg by (dimension_ModelId, region) (aws_bedrock_invocation_latency_average${selector})`,
    },
  ];
}
