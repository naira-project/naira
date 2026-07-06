import { useState } from 'react';
import { useParams, useNavigate } from 'react-router';
import Markdown from 'react-markdown';
import { useModelWithId } from '../hooks/useModels';
import { cn } from '../lib/utils';
import Stats from './Stats';
import CatalogGraph from './CatalogGraph';

export default function ModelSpec() {
  const navigate = useNavigate();
  const { id = '' } = useParams();
  const { model, loading, error } = useModelWithId(decodeURIComponent(decodeURIComponent(id)));
  const [source, setSource] = useState<'Model docs' | 'Stats' | 'Graph'>('Model docs');

  const tabs = ['Model docs', 'Stats', 'Graph'] as const;

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      {loading && <p className="text-sm text-muted-foreground">Loading…</p>}
      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className="flex flex-col flex-1 overflow-hidden">
        {!loading && !error && (
          <div className="flex flex-col gap-2 px-3 py-2 border-b shrink-0">
            <button
                onClick={() => navigate('/model-registry')}
                className="text-sm text-muted-foreground hover:text-foreground transition-colors self-start"
            >
                ← Model Registries
            </button>
            <h1 className="text-2xl font-medium truncate" title={model?.name}>
                {model?.name}
            </h1>

            <div className="flex-1 overflow-y-auto px-3 py-2">
              <div className="flex gap-1 mb-2">
                {tabs.map((tab) => (
                  <button
                    key={tab}
                    onClick={() => setSource(tab)}
                    className={cn(
                      'px-3 py-1.5 rounded-md text-sm cursor-pointer transition-colors',
                      source === tab
                        ? 'bg-primary text-primary-foreground font-semibold'
                        : 'bg-accent text-foreground hover:bg-accent/70'
                    )}
                  >
                    {tab}
                  </button>
                ))}
              </div>

              <div className="text-sm whitespace-pre-wrap">
                {source === 'Model docs'
                  ? <div className="prose prose-sm max-w-none prose-headings:text-foreground prose-p:text-foreground prose-headings:mb-1 prose-headings:mt-4 prose-p:my-1 prose-ul:my-1 prose-li:my-0"><Markdown>{model?.description ?? ''}</Markdown></div>
                  : source === 'Stats'
                  ? <Stats adopters={model?.adopters ?? []} />
                  : model
                  ? <CatalogGraph rootNode={{ name: model.resourceName, label: model.name }} />
                  : null}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
