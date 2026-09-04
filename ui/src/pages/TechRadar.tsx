import { Badge } from '@/components/ui/badge';
import { TECH_RADAR_PLUGIN } from '@/lib/techRadar';
import QuadrantSummaryCard from '../components/radar/QuadrantSummaryCard';
import RadarChart from '../components/radar/RadarChart';
import RingLegend from '../components/radar/RingLegend';
import PluginSyncState from '../components/states/PluginSyncState';
import { useTechRadar } from '../hooks/useTechRadar';

/**
 * Radar overview: the classic circle plus per-quadrant summary cards and the
 * ring definitions from the admin-managed radar configuration.
 */
export default function TechRadar() {
  const { model, loading, error } = useTechRadar();

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3">
          <h1 className="text-lg font-semibold text-foreground">{model?.title ?? 'Tech Radar'}</h1>
          {model?.edition && <Badge variant="outline">{model.edition}</Badge>}
          <div className="flex-1" />
          {model && (
            <span className="text-sm text-muted-foreground">
              {model.movedCount} moved · owned by {model.owner}
            </span>
          )}
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading tech radar…</p>
          ) : error ? (
            <p className="text-sm text-destructive">{error}</p>
          ) : !model ? (
            <PluginSyncState pluginNames={[TECH_RADAR_PLUGIN]} />
          ) : (
            <>
              {model.orphans.length > 0 && (
                <p className="mb-3 rounded-md bg-orange-50 px-3 py-2 text-xs text-orange-700">
                  {model.orphans.length}{' '}
                  {model.orphans.length === 1 ? 'entry references' : 'entries reference'} a quadrant
                  or ring missing from the taxonomy and {model.orphans.length === 1 ? 'is' : 'are'}{' '}
                  hidden. Re-run the sync after fixing the radar configuration.
                </p>
              )}

              <div className="flex flex-col gap-6 lg:flex-row">
                <RadarChart model={model} />
                <div className="shrink-0 lg:w-48">
                  <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Rings
                  </h2>
                  <RingLegend rings={model.rings} />
                </div>
              </div>

              <div className="mt-8 grid grid-cols-1 gap-4 md:grid-cols-2">
                {model.quadrants.map((quadrant) => (
                  <QuadrantSummaryCard key={quadrant.id} quadrant={quadrant} model={model} />
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
