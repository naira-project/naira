import type { NodeResource } from '../lib/catalogApi';
import PropertiesPanel from './PropertiesPanel';

interface PluginPropertiesPanelsProps {
  node: NodeResource;
}

/**
 * Renders a node's properties with one section per plugin that reported them,
 * in the order the API returns the claims. The same key reported by two plugins
 * shows both values, each under its source plugin.
 */
export default function PluginPropertiesPanels({ node }: PluginPropertiesPanelsProps) {
  // A plugin can claim a node without reporting any properties; an empty
  // section for it would only add noise.
  const claims = (node.pluginClaims ?? []).filter((claim) => Object.keys(claim.props).length > 0);

  if (claims.length === 0) {
    return <PropertiesPanel props={{}} />;
  }

  return (
    <div className="flex flex-col gap-4">
      {claims.map((claim) => (
        <PropertiesPanel
          key={claim.plugin}
          props={claim.props}
          title={`${node.kind} Properties · ${claim.plugin}`}
        />
      ))}
    </div>
  );
}
