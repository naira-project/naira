import { AlertCircle, ExternalLink } from 'lucide-react';
import { useMemo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { useCatalogGraph } from '@/hooks/useCatalogGraph';
import type { NodeResource } from '@/lib/catalogApi';
import {
  formatCost,
  formatCount,
  formatLatency,
  formatRelativeTime,
  formatScore,
  formatWindow,
  type LangfuseMetrics as LangfuseMetricsModel,
  langfuseMetricsFromGraph,
} from '@/lib/langfuseMetrics';

interface LangfuseMetricsProps {
  node: NodeResource;
}

/**
 * The detail page's Langfuse tab. Owns its own data so the page stays
 * kind-agnostic, and reads the same depth-1 graph slice as the Graph tab so
 * both share one cached request.
 */
export default function LangfuseMetrics({ node }: LangfuseMetricsProps) {
  const { graph, loading, error } = useCatalogGraph({ name: node.name }, 1);
  const scopes = useMemo(() => langfuseMetricsFromGraph(graph, node.name), [graph, node.name]);

  if (loading) {
    return <p className="text-sm text-muted-foreground">Loading metrics…</p>;
  }

  if (error) {
    return <p className="text-sm text-red-500">{error}</p>;
  }

  if (scopes.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No Langfuse project is linked to this application. Bind it in the langfuse plugin's
        configuration, then run the plugin to pull in its cost, latency and feedback metrics.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {scopes.map((scope) => (
        <ScopeCard key={scope.name} scope={scope} />
      ))}
    </div>
  );
}

function ScopeCard({ scope }: { scope: LangfuseMetricsModel }) {
  const windowLabel = formatWindow(scope.window);
  const collected = formatRelativeTime(scope.collectedAt);
  const staleSince = formatRelativeTime(scope.staleSince);

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <div className="flex flex-wrap items-baseline gap-2">
            <h3 className="text-sm font-semibold text-foreground">
              {scope.scope || 'Whole project'}
            </h3>
            {/* The catalog keeps one snapshot, so the header carries the
                window and the age; a number alone cannot be read. */}
            {windowLabel && <span className="text-xs text-muted-foreground">{windowLabel}</span>}
            {collected && (
              <span className="text-xs text-muted-foreground" title={scope.collectedAt}>
                · updated {collected}
              </span>
            )}
          </div>

          {/* Status never rides on color alone — the icon and the word carry it. */}
          {scope.stale && (
            <span
              className="inline-flex items-center gap-1 rounded-full border border-orange-300 bg-orange-50 px-2.5 py-0.5 text-xs font-semibold text-orange-700"
              title={scope.lastError || undefined}
            >
              <AlertCircle size={12} />
              Stale{staleSince ? ` · since ${staleSince}` : ''}
            </span>
          )}
        </div>

        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <StatTile label="Total cost" value={formatCost(scope.totalCostUsd)} />
          <StatTile
            label="Avg latency"
            value={formatLatency(scope.avgLatency, scope.latencyUnit)}
          />
          <StatTile label="Generations" value={formatCount(scope.observationCount)} />
          <StatTile
            label={scope.scoreName ? scoreLabel(scope.scoreName) : 'Feedback'}
            value={formatScore(scope.scoreMean)}
            detail={detailForScore(scope)}
          />
        </div>

        {scope.stale && scope.lastError && (
          <p className="text-xs text-muted-foreground">
            Last sync failed: <span className="font-mono">{scope.lastError}</span>
          </p>
        )}

        {scope.tracesUrl && (
          <a
            href={scope.tracesUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex w-fit items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
          >
            View traces in Langfuse
            <ExternalLink size={14} />
          </a>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * A single metric. A null `value` means the window held no data, which is not
 * zero and must not be shown as one. No tabular-nums: that is for columns
 * that align vertically, and gives a standalone number gappy digits.
 */
function StatTile({
  label,
  value,
  detail,
}: {
  label: string;
  value: string | null;
  detail?: string;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span
        className={
          value === null
            ? 'text-xl font-semibold text-muted-foreground'
            : 'text-xl font-semibold text-foreground'
        }
      >
        {value ?? '—'}
      </span>
      {value === null ? (
        <span className="text-[0.65rem] text-muted-foreground">No data in window</span>
      ) : (
        detail && <span className="text-[0.65rem] text-muted-foreground">{detail}</span>
      )}
    </div>
  );
}

/** Turns a Langfuse score name such as "user_feedback" into a tile label. */
function scoreLabel(name: string): string {
  const spaced = name.replace(/[_-]+/g, ' ').trim();
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function detailForScore(scope: LangfuseMetricsModel): string | undefined {
  const count = formatCount(scope.scoreCount);
  if (!count) {
    return undefined;
  }
  return `${count} ${scope.scoreCount === '1' ? 'rating' : 'ratings'}`;
}
