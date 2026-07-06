import { useState, useEffect } from 'react';
import { buildEqualityFilter, fetchNode, fetchNodeByName, fetchNodes, fetchRelations, NodeResource } from '../lib/catalogApi';
import { useOpenMFPContext } from './useOpenMFPContext';

interface Model {
  id: string;
  resourceName: string;
  name: string;
  source: string;
  type: string;
  description: string;
  metadata: Metadata;
  adopters: AdoptersType[];
}

interface Metadata {
  owned_by: string;
  raw: Raw;
}

interface Raw {
  created: number;
  id: string;
  object: string;
  owned_by: string;
}

export type AdoptersType = {
  name: string;
  namespace: string;
  team: string;
  litellmVirtualKey: string;
}

function sourceFromPath(path: string) {
  return path.split('/')[0] ?? '';
}

function nameFromPath(path: string) {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

function mapNodeToModel(node: NodeResource): Model {
  const props = node.props ?? {};
  const source = props.source || sourceFromPath(node.path);

  return {
    id: node.path,
    resourceName: node.name,
    name: nameFromPath(node.path),
    source,
    type: props.type || '',
    description: props.description || '',
    metadata: {
      owned_by: props.owned_by || '',
      raw: {
        created: props.creation_timestamp || 0,
        id: node.path,
        object: node.kind,
        owned_by: props.owned_by || '',
      },
    },
    adopters: [],
  };
}

function mapApplicationNodeToAdopter(node: NodeResource): AdoptersType {
  const props = node.props ?? {};

  return {
    name: nameFromPath(node.path),
    namespace: props.namespace || '',
    team: props.team || '',
    litellmVirtualKey: props.litellm_virtual_key || '',
  };
}

export function useModels(source: 'all' | 'mlflow' | 'litellm') {
  const { token, isReady } = useOpenMFPContext();
  const [models, setModels] = useState<Model[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isReady) {
      return;
    }

    setLoading(true);
    setError(null);
    fetchNodes(token, {
      filter: buildEqualityFilter('kind', 'model'),
      pageSize: 1000,
    })
      .then((nodes) => {
        const nextModels = nodes.map(mapNodeToModel);
        setModels(nextModels);
        setLoading(false);
      })
      .catch(() => { setError('Failed to load models'); setLoading(false); });
  }, [source, token, isReady]);

  return { models, loading, error };
}

export function useModelWithId(id: string) {
  const { token, isReady } = useOpenMFPContext();
  const [model, setModel] = useState<Model | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isReady) {
      return;
    }

    setLoading(true);
    setError(null);
    const loadModel = async () => {
      const node = await fetchNode(token, 'model', id);
      const nextModel = mapNodeToModel(node);

      const relations = await fetchRelations(token, {
        filter: buildEqualityFilter('toNode', nextModel.resourceName),
        pageSize: 1000,
      });

      const adopterNames = relations
        .filter((relation) => relation.kind === 'uses_model')
        .map((relation) => relation.fromNode)
        .filter(Boolean);

      if (adopterNames.length > 0) {
        const adopterNodes = await Promise.all(
          adopterNames.map(async (name) => {
            try {
              return await fetchNodeByName(token, name);
            } catch {
              return null;
            }
          })
        );

        nextModel.adopters = adopterNodes
          .filter((entry): entry is NodeResource => entry !== null)
          .map(mapApplicationNodeToAdopter);
      }

      setModel(nextModel);
      setLoading(false);
    };

    loadModel()
      .catch(() => { setError('Failed to load models'); setLoading(false); });
  }, [id, token, isReady]);

  return {model, loading, error};
}