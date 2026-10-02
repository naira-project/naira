import type { CatalogGraphNode } from '../../hooks/useCatalogGraph';

/**
 * Region and short ID badges for inference endpoints. When connecting in the graph 
 * inference endpoints have the same display name. To differentiate different inference endpoints,
 * their regions or beginning of their inference endpoint ID are provided as a chip under the label node.
 */
export default function InferenceEndpointDetails({ node }: { node: CatalogGraphNode }) {
  const region =
    typeof node.properties?.region_name === 'string'
        ? node.properties.region_name
        : undefined;

  const id = typeof node.properties?.id === 'string' ? node.properties.id : undefined;

  if (!region && !id) {
    return null;
  }

  return (
    <div className="flex flex-wrap gap-1">
      {region && (
        <span
          className="rounded bg-black/5 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
          title={`Region: ${region}`}
        >
          {region}
        </span>
      )}
      {id && (
        <span
          className="rounded bg-black/5 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
          title={`ID: ${id}`}
        >
          {id.slice(0, 8)}
        </span>
      )}
    </div>
  );
}
