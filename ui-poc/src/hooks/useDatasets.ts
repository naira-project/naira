import { useEffect, useState } from 'react';
import { buildEqualityFilter, fetchNodes, fetchRelations, NodeResource } from '../lib/catalogApi';

export interface DatasetColumn {
  name: string;
  type: string;
  nativeType?: string;
  description?: string;
}

export interface Dataset {
  id: string;
  resourceName: string;
  name: string;
  source: string;
  platform: string;
  fqn: string;
  description: string;
  tags: string[];
  owners: string[];
  columns: DatasetColumn[];
  sourceUrl: string;
  derivedFrom: number;
  feeds: number;
}

function nameFromPath(path: string) {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

function splitList(value: unknown): string[] {
  if (typeof value !== 'string' || value.trim() === '') {
    return [];
  }
  return value.split(',').map((entry) => entry.trim()).filter(Boolean);
}

function parseColumns(value: unknown): DatasetColumn[] {
  if (typeof value !== 'string' || value.trim() === '') {
    return [];
  }
  try {
    const parsed = JSON.parse(value);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.map((column: any) => ({
      name: String(column.name ?? ''),
      type: String(column.type ?? ''),
      nativeType: column.native_type ? String(column.native_type) : undefined,
      description: column.description ? String(column.description) : undefined,
    }));
  } catch {
    return [];
  }
}

function mapNodeToDataset(node: NodeResource): Dataset {
  const props = node.props ?? {};

  return {
    id: node.path,
    resourceName: node.name,
    name: nameFromPath(node.path),
    source: props.source || '',
    platform: props.platform || '',
    fqn: props.fqn || '',
    description: props.description || '',
    tags: splitList(props.tags),
    owners: splitList(props.owners),
    columns: parseColumns(props.columns_json),
    sourceUrl: props.source_url || '',
    derivedFrom: 0,
    feeds: 0,
  };
}

export function useDatasets() {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    const load = async () => {
      const [nodes, relations] = await Promise.all([
        fetchNodes({ filter: buildEqualityFilter('kind', 'dataset'), pageSize: 1000 }),
        fetchRelations({ filter: buildEqualityFilter('kind', 'derived_from'), pageSize: 1000 }),
      ]);

      const byName = new Map<string, Dataset>();
      const list = nodes.map((node) => {
        const dataset = mapNodeToDataset(node);
        byName.set(dataset.resourceName, dataset);
        return dataset;
      });

      // A derived_from relation points From the downstream dataset To the
      // upstream source it is derived from.
      for (const relation of relations) {
        const downstream = byName.get(relation.fromNode);
        if (downstream) {
          downstream.derivedFrom += 1;
        }
        const upstream = byName.get(relation.toNode);
        if (upstream) {
          upstream.feeds += 1;
        }
      }

      if (!cancelled) {
        setDatasets(list);
        setLoading(false);
      }
    };

    load().catch(() => {
      if (!cancelled) {
        setError('Failed to load datasets');
        setLoading(false);
      }
    });

    return () => {
      cancelled = true;
    };
  }, []);

  return { datasets, loading, error };
}
