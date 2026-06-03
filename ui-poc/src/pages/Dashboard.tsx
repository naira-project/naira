import React, { useState } from 'react';
import { Bell, Search, AlertTriangle, Clock, Check, RefreshCw } from 'lucide-react';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Badge } from '../components/ui/badge';
import { Avatar, AvatarFallback } from '../components/ui/avatar';
import { Tooltip, TooltipProvider } from '../components/ui/tooltip';
import { WORK_ITEMS, STAT_CARDS } from '../constants/Constants';

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { variant: 'success' | 'warning' | 'default'; icon: React.ReactNode }> = {
    Done: { variant: 'success', icon: <Check size={10} /> },
    'In Progress': { variant: 'warning', icon: <Clock size={10} /> },
    'Pending Review': { variant: 'default', icon: <Clock size={10} /> },
  };
  const { variant, icon } = map[status] ?? { variant: 'default', icon: null };
  return (
    <Badge variant={variant}>
      {icon}
      {status}
    </Badge>
  );
}

function PriorityDot({ priority }: { priority: string }) {
  const colors: Record<string, string> = {
    High: 'bg-red-400',
    Medium: 'bg-orange-400',
    Low: 'bg-green-400',
  };
  return <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${colors[priority] ?? 'bg-gray-400'}`} />;
}

function SyncCatalogButton() {
  const [syncing, setSyncing] = useState(false);
  const [syncMessage, setSyncMessage] = useState<string | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  const handleSync = async () => {
    setSyncing(true);
    setSyncMessage(null);
    setSyncError(null);

    try {
      const response = await fetch('/v1/plugins:run', { method: 'POST' });
      const payload = await response.json();

      if (!response.ok) {
        throw new Error(payload.error ?? 'Failed to synchronize data');
      }

      const results = Array.isArray(payload.results) ? payload.results : [];
      const errorCount = results.filter((result: { error?: string }) => typeof result.error === 'string' && result.error.length > 0).length;
      const successCount = results.length - errorCount;
      setSyncMessage(errorCount > 0 ? `Synced ${successCount} plugin(s), ${errorCount} error(s)` : `Synced ${successCount} plugin(s)`);
    } catch (error) {
      setSyncError(error instanceof Error ? error.message : 'Failed to synchronize data');
    } finally {
      setSyncing(false);
    }
  };

  return (
    <>
      <button
        onClick={handleSync}
        disabled={syncing}
        className="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <RefreshCw size={16} className={syncing ? 'animate-spin' : ''} />
        {syncing ? 'Synchronizing…' : 'Synchronize Data'}
      </button>
      {syncMessage && <Badge variant="success">{syncMessage}</Badge>}
      {syncError && <Badge variant="error">{syncError}</Badge>}
    </>
  );
}

export default function Dashboard() {
  const [search, setSearch] = useState('');

  return (
    <TooltipProvider>
      <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
        <div className="flex flex-1 flex-col overflow-hidden">

          {/* Top bar */}
          <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
            <Input
              startAdornment={<Search size={16} />}
              placeholder="Search models, experiments, pipelines…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="max-w-[480px]"
            />
            <SyncCatalogButton />
            <div className="flex-1" />
            <Tooltip content="Notifications">
              <button className="relative rounded-full p-1.5 hover:bg-gray-100 dark:hover:bg-gray-700">
                <Bell size={20} className="text-gray-600 dark:text-gray-300" />
                <span className="absolute -right-0.5 -top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] font-bold text-white">
                  3
                </span>
              </button>
            </Tooltip>
            <Tooltip content="Kaan Kocak">
              <button>
                <Avatar>
                  <AvatarFallback>KK</AvatarFallback>
                </Avatar>
              </button>
            </Tooltip>
          </header>

          {/* Scrollable content */}
          <main className="flex flex-1 flex-col gap-6 overflow-y-auto px-6 py-6">

            {/* Alert card */}
            <Card className="border-l-4 border-l-primary bg-gradient-to-br from-primary/5 to-secondary/5">
              <CardContent className="flex items-start gap-3 py-3.5">
                <AlertTriangle size={20} className="mt-0.5 shrink-0 text-primary" />
                <div>
                  <p className="text-sm font-semibold text-primary">
                    Model drift detected — Recommendation Engine v3
                  </p>
                  <p className="mt-0.5 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                    PSI score exceeded threshold (0.23 &gt; 0.20) on feature{' '}
                    <strong>user_session_duration</strong>. Consider retraining or rolling back to v2.
                  </p>
                </div>
                <Badge variant="error" className="ml-auto shrink-0">Critical</Badge>
              </CardContent>
            </Card>

            {/* Stats */}
            <section>
              <h2 className="mb-3 text-sm font-semibold text-foreground dark:text-foreground-dark-default">Overview</h2>
              <div className="grid grid-cols-4 gap-4">
                {STAT_CARDS.map(({ label, value, delta, color }) => (
                  <Card key={label}>
                    <CardContent className="pb-4">
                      <p className="text-[0.65rem] font-medium uppercase tracking-wider text-foreground-secondary dark:text-foreground-dark-secondary">
                        {label}
                      </p>
                      <p className="my-1 text-3xl font-bold" style={{ color }}>
                        {value}
                      </p>
                      <p className="text-xs text-foreground-secondary dark:text-foreground-dark-secondary">{delta}</p>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </section>

            {/* Work items */}
            <section>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">My Work Items</h2>
                <Badge variant="outline">
                  {WORK_ITEMS.filter((w) => w.status === 'In Progress').length} in progress
                </Badge>
              </div>
              <Card>
                <ul className="divide-y divide-gray-200 dark:divide-gray-700">
                  {WORK_ITEMS.map((item) => (
                    <li
                      key={item.id}
                      className="flex cursor-pointer items-center gap-3 px-5 py-3.5 hover:bg-gray-50 dark:hover:bg-white/5"
                    >
                      <PriorityDot priority={item.priority} />
                      <span className="min-w-[56px] font-mono text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                        {item.id}
                      </span>
                      <span className="flex-1 text-sm font-medium text-foreground dark:text-foreground-dark-default">
                        {item.title}
                      </span>
                      <StatusBadge status={item.status} />
                    </li>
                  ))}
                </ul>
              </Card>
            </section>

          </main>
        </div>
      </div>
    </TooltipProvider>
  );
}
