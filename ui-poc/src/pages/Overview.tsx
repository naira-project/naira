import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Search } from 'lucide-react';
import { Input } from '../components/ui/input';
import { Card, CardContent } from '../components/ui/card';
import { CATALOG_VIEWPOINTS } from '../config/viewpoints';

/**
 * Landing page listing every catalog viewpoint as a clickable card.
 */
export default function Overview() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');

  const filteredViewpoints = CATALOG_VIEWPOINTS.filter((viewpoint) =>
    viewpoint.heading.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="flex h-screen overflow-hidden bg-background dark:bg-background-dark-default">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-white px-6 py-3 dark:border-gray-700 dark:bg-background-dark-paper">
          <Input
            startAdornment={<Search size={16} />}
            placeholder="Search viewpoints..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="max-w-[320px]"
          />

          <div className="flex-1" />
        </header>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          <div className="mb-6 text-center">
            <h1 className="text-xl font-semibold text-foreground dark:text-foreground-dark-default">
              Overview
            </h1>
            <p className="mt-1 mb-4 text-sm text-foreground-secondary dark:text-foreground-dark-secondary">
              Select a viewpoint to browse its catalog.
            </p>
          </div>

          <div className="ml-12 grid grid-cols-[repeat(auto-fill,16rem)] gap-4">
            {filteredViewpoints.map((viewpoint) => (
              <Card
                key={viewpoint.path}
                className="cursor-pointer transition-shadow hover:shadow-md"
              >
                <CardContent>
                  <h3 className="text-sm font-semibold text-foreground dark:text-foreground-dark-default">
                    {viewpoint.heading}
                  </h3>
                  <p className="mt-1 text-xs text-foreground-secondary dark:text-foreground-dark-secondary">
                    {viewpoint.subheading}
                  </p>
                </CardContent>
                <div className="px-4 pb-4">
                  <button
                    onClick={() => navigate(`/catalog/${viewpoint.path}`)}
                    className="flex w-full items-center justify-center rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
                  >
                    Browse
                  </button>
                </div>
              </Card>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
