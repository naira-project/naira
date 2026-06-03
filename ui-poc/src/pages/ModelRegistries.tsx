import { useState } from 'react';
import { Search, ChevronDown, ChevronUp, Info } from 'lucide-react';
//import LuigiClient from '@luigi-project/client';
import { Input } from '../components/ui/input';
import { Badge } from '../components/ui/badge';
import { Slider } from '../components/ui/slider';
import { useModels } from '../hooks/useModels';
import { MODEL_KIND, MODEL_TYPE, RISK, DOMAIN, CATEGORIES } from '../constants/Constants';
import { cn } from '../lib/utils';
import { useNavigate } from 'react-router';


interface Model {
  id: string;
  name: string;
  source: string;
  type: string;
  description: string;
  metadata: { owned_by: string; raw: { created: number; id: string; object: string; owned_by: string } };
}

const COL_WIDTHS = '20% 10% 13% 10% 10% 11% 11% 15%';

function ModelTableHeader() {
  return (
    <div
      className="grid rounded-t-[6px] border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-700 dark:bg-white/5"
      style={{ gridTemplateColumns: COL_WIDTHS }}
    >
      {CATEGORIES.map((col) => (
        <span key={col} className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
          {col}
        </span>
      ))}
    </div>
  );
}

function ModelRow({ model }: { model: Model }) {
  const navigate = useNavigate();

  return (
    <div
      className="grid items-center border-b border-gray-200 px-4 py-3 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-white/5"
      style={{ gridTemplateColumns: COL_WIDTHS }}
    >
      <span className="truncate text-sm font-medium text-foreground dark:text-foreground-dark-default" title={model.name}>
        {model.name}
      </span>
      <span className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">—</span>
      <span className="truncate text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
        {model.type || '—'}
      </span>
      <button
        onClick={() => navigate(`/model-registry/${encodeURIComponent(encodeURIComponent(model.id))}`)}
        className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-foreground-secondary hover:bg-gray-100 hover:text-foreground dark:text-foreground-dark-secondary dark:hover:bg-white/10 dark:hover:text-foreground-dark-default transition-colors"
        title="View details"
      >
        <Info size={14} />
        Details
      </button>
    </div>
  );
}

function SourceGroup({ label, models }: { label: string; models: Model[] }) {
  const [open, setOpen] = useState(true);

  return (
    <div className="mb-4">
      <button
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'flex w-full items-center gap-2 border border-gray-200 bg-white px-4 py-2.5 text-left hover:bg-gray-50 dark:border-gray-700 dark:bg-background-dark-paper dark:hover:bg-white/5',
          open ? 'rounded-t-[6px]' : 'rounded-[6px]'
        )}
      >
        {open ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        <span className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">{label}</span>
        <span className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">({models.length})</span>
      </button>

      {open && (
        <div className="overflow-hidden rounded-b-[6px] border border-t-0 border-gray-200 dark:border-gray-700">
          <ModelTableHeader />
          {models.length === 0 ? (
            <p className="px-4 py-3 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">No models found.</p>
          ) : (
            models.map((m) => <ModelRow key={m.id} model={m} />)
          )}
        </div>
      )}
    </div>
  );
}

type SourceTab = 'all' | 'mlflow' | 'litellm';

const TABS: { value: SourceTab; label: string }[] = [
  { value: 'all', label: 'All Models' },
  { value: 'mlflow', label: 'MLflow' },
  { value: 'litellm', label: 'LiteLLM' },
];

export default function ModelRegistries() {
  const [search, setSearch] = useState('');
  const [modelSize, setModelSize] = useState<number[]>([0, 700]);
  const [source, setSource] = useState<SourceTab>('all');
  const { models, loading, error } = useModels(source);

  const filtered = models.filter(
    (m) =>
      (source === 'all' || m.source === source) &&
      (!search || m.name.toLowerCase().includes(search.toLowerCase()))
  );

  const mlflowModels = filtered.filter((m) => m.source === 'mlflow');
  const litellmModels = filtered.filter((m) => m.source === 'litellm');

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">

        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <Input
            startAdornment={<Search size={16} />}
            placeholder="Search models by name, tag or description..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="max-w-[480px]"
          />
        </header>

        <div className="flex flex-1 flex-col overflow-hidden">

          {/* Filters */}
          <div className="flex shrink-0 flex-wrap items-start gap-6 border-b border-gray-200 px-6 py-4 dark:border-gray-700">
            {[
              { label: 'Model Kind', values: MODEL_KIND },
              { label: 'Model Type', values: MODEL_TYPE },
              { label: 'Risk', values: RISK },
              { label: 'Domain', values: DOMAIN },
            ].map(({ label, values }) => (
              <div key={label} className="min-w-[140px]">
                <p className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
                  {label}
                </p>
                <div className="mt-2 flex flex-wrap gap-1">
                  {values.map((v) => (
                    <Badge key={v} variant="secondary" className="cursor-pointer text-[0.7rem]">{v}</Badge>
                  ))}
                </div>
              </div>
            ))}

            <div className="min-w-[200px] flex-1">
              <p className="text-[0.65rem] font-semibold uppercase tracking-wide text-foreground-secondary dark:text-foreground-dark-secondary">
                Model Size (B Parameters)
              </p>
              <p className="mt-1 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                {modelSize[0]}B – {modelSize[1] === 700 ? '700B+' : `${modelSize[1]}B`}
              </p>
              <Slider
                value={modelSize}
                onValueChange={setModelSize}
                min={0}
                max={700}
                className="mt-2"
              />
            </div>
          </div>

          {/* Table area */}
          <div className="flex-1 overflow-y-auto px-6 py-4">

            {/* Source tabs */}
            <div className="mb-4 flex gap-2">
              {TABS.map(({ value, label }) => (
                <button
                  key={value}
                  onClick={() => setSource(value)}
                  className={cn(
                    'rounded-lg px-4 py-1.5 text-sm transition-colors',
                    source === value
                      ? 'bg-primary font-semibold text-white'
                      : 'bg-gray-100 font-normal text-foreground hover:bg-gray-200 dark:bg-white/10 dark:text-foreground-dark-default dark:hover:bg-white/20'
                  )}
                >
                  {label}
                </button>
              ))}
            </div>

            {loading && <p className="text-sm text-foreground-secondary dark:text-foreground-dark-secondary">Loading…</p>}
            {error && <p className="text-sm text-red-500">{error}</p>}

            {!loading && !error && source === 'all' && (
              <>
                <SourceGroup label="MLflow" models={mlflowModels} />
                <SourceGroup label="LiteLLM" models={litellmModels} />
              </>
            )}

            {!loading && !error && source !== 'all' && (
              <div className="overflow-hidden rounded-[6px] border border-gray-200 dark:border-gray-700">
                <ModelTableHeader />
                {filtered.length === 0 ? (
                  <p className="px-4 py-3 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">No models found.</p>
                ) : (
                  filtered.map((m) => <ModelRow key={m.id} model={m} />)
                )}
              </div>
            )}

          </div>
        </div>
      </div>
    </div>
  );
}
