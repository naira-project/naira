import { render, screen, within } from '@testing-library/react';
import type { NodeResource } from '../lib/catalogApi';
import PluginPropertiesPanels from './PluginPropertiesPanels';

function modelNode(pluginClaims: NodeResource['pluginClaims']): NodeResource {
  return {
    name: 'nodes/model/clusterA/deepseek',
    kind: 'model',
    path: 'clusterA/deepseek',
    pluginClaims,
  };
}

test('shows the same key from two plugins under each source, in API order', () => {
  render(
    <PluginPropertiesPanels
      node={modelNode([
        { plugin: 'mlflow/clusterA', props: { release: '2.34', token_price: '$10' } },
        { plugin: 'litellm', props: { token_price: '$5' } },
      ])}
    />,
  );

  const headings = screen.getAllByRole('heading', { level: 3 });
  expect(headings.map((heading) => heading.textContent)).toEqual([
    'model Properties · mlflow/clusterA',
    'model Properties · litellm',
  ]);

  const [mlflowSection, litellmSection] = headings.map(
    (heading) => heading.parentElement as HTMLElement,
  );
  expect(within(mlflowSection).getByText('$10')).toBeInTheDocument();
  expect(within(mlflowSection).getByText('2.34')).toBeInTheDocument();
  expect(within(litellmSection).getByText('$5')).toBeInTheDocument();
  expect(within(litellmSection).queryByText('release')).not.toBeInTheDocument();
});

test('skips plugins that reported no properties', () => {
  render(
    <PluginPropertiesPanels
      node={modelNode([
        { plugin: 'openmetadata', props: {} },
        { plugin: 'litellm', props: { token_price: '$5' } },
      ])}
    />,
  );

  expect(screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent)).toEqual([
    'model Properties · litellm',
  ]);
});

test('shows a single empty state when no plugin reported properties', () => {
  render(<PluginPropertiesPanels node={modelNode([{ plugin: 'openmetadata', props: {} }])} />);

  expect(screen.getByText('No properties.')).toBeInTheDocument();
  expect(screen.queryByRole('heading')).not.toBeInTheDocument();
});
