import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import RadarEntryTable from '../components/radar/RadarEntryTable';
import PluginSyncState from '../components/states/PluginSyncState';
import { useTechRadar } from '../hooks/useTechRadar';
import { TECH_RADAR_PLUGIN } from '../lib/techRadar';

/**
 * Quadrant detail: all of one quadrant's entries as a table, filterable by
 * ring and by movement.
 */
export default function TechRadarQuadrant() {
  const { quadrantId = '' } = useParams();
  const navigate = useNavigate();
  const { model, loading, error } = useTechRadar();

  const [activeRing, setActiveRing] = useState<string | null>(null);
  const [movedOnly, setMovedOnly] = useState(false);

  const quadrant = model?.quadrants.find((q) => q.id === quadrantId) ?? null;
  const entries = (model?.entries ?? []).filter(
    (entry) =>
      entry.quadrant === quadrantId &&
      (activeRing === null || entry.ring === activeRing) &&
      (!movedOnly || entry.moved !== 'none'),
  );

  const pillClass = (active: boolean) =>
    cn(
      'rounded-lg px-4 py-1.5 text-sm transition-colors',
      active ? 'bg-primary text-white' : 'bg-gray-100 text-muted-foreground hover:bg-gray-200',
    );

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3">
          <Button variant="ghost" size="sm" onClick={() => navigate('/tech-radar')}>
            <ArrowLeft size={16} />
            Radar
          </Button>
          <h1 className="text-lg font-semibold text-foreground">
            {quadrant?.name ?? 'Unknown quadrant'}
          </h1>
          <div className="flex-1" />
          {model?.edition && <span className="text-sm text-muted-foreground">{model.edition}</span>}
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading tech radar…</p>
          ) : error ? (
            <p className="text-sm text-destructive">{error}</p>
          ) : !model ? (
            <PluginSyncState pluginNames={[TECH_RADAR_PLUGIN]} />
          ) : !quadrant ? (
            <p className="text-sm text-muted-foreground">
              This quadrant does not exist in the current radar configuration.
            </p>
          ) : (
            <div className="flex flex-col gap-3">
              <div className="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  onClick={() => setActiveRing(null)}
                  className={pillClass(activeRing === null)}
                >
                  All rings
                </button>
                {model.rings.map((ring) => (
                  <button
                    type="button"
                    key={ring.id}
                    onClick={() => setActiveRing(activeRing === ring.id ? null : ring.id)}
                    className={pillClass(activeRing === ring.id)}
                  >
                    {ring.name}
                  </button>
                ))}
                <div className="mx-2 h-5 w-px bg-gray-200" />
                <button
                  type="button"
                  onClick={() => setMovedOnly((value) => !value)}
                  className={pillClass(movedOnly)}
                >
                  Moved only
                </button>
              </div>

              <RadarEntryTable entries={entries} rings={model.rings} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
