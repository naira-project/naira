import { Search } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { CATALOG_VIEWPOINTS } from '../config/viewpoints';

/**
 * Standalone pages shown alongside the catalog viewpoints. Unlike viewpoints
 * these have no kind selector or plugin tabs — they route to their own page.
 */
const EXTRA_PAGES = [
  {
    path: '/tech-radar',
    heading: 'Tech Radar',
    subheading: 'Which technologies to adopt, trial, assess, or hold.',
  },
];

/**
 * Landing page listing every catalog viewpoint as a clickable card.
 */
export default function Overview() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');

  const cards = [
    ...CATALOG_VIEWPOINTS.map((viewpoint) => ({
      path: `/catalog/${viewpoint.path}`,
      heading: viewpoint.heading,
      subheading: viewpoint.subheading,
    })),
    ...EXTRA_PAGES,
  ];

  const filteredCards = cards.filter((card) =>
    card.heading.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex shrink-0 items-center gap-3 border-b border-gray-200 bg-card px-6 py-3">
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
            <h1 className="text-xl font-semibold text-foreground">Overview</h1>
            <p className="mt-1 mb-4 text-sm text-muted-foreground">
              Select a viewpoint to browse its catalog.
            </p>
          </div>

          <div className="ml-12 grid grid-cols-[repeat(auto-fill,16rem)] gap-4">
            {filteredCards.map((card) => (
              <Card key={card.path} className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent>
                  <h3 className="text-sm text-center font-semibold text-foreground">
                    {card.heading}
                  </h3>
                  <p className="mt-1 text-xs text-center text-muted-foreground">
                    {card.subheading}
                  </p>
                </CardContent>
                <div className="px-4 pb-4">
                  <button
                    type="button"
                    onClick={() => navigate(card.path)}
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
