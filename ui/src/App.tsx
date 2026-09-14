import { QueryClientProvider } from '@tanstack/react-query';
import { lazy, Suspense } from 'react';
import { Route, BrowserRouter as Router, Routes } from 'react-router';
import { CATALOG_VIEWPOINTS } from './config/viewpoints';
import { queryClient } from './lib/queryClient';

const CatalogDetail = lazy(() => import('./pages/CatalogDetail'));
const CatalogView = lazy(() => import('./pages/CatalogView'));
const Overview = lazy(() => import('./pages/Overview'));
const PluginsPage = lazy(() => import('./pages/PluginsPage'));

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Router>
        <Suspense fallback={<p>Loading…</p>}>
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/catalog" element={<Overview />} />
            <Route path="/plugins" element={<PluginsPage />} />
            {CATALOG_VIEWPOINTS.map((viewpoint) => (
              <Route
                key={viewpoint.path}
                path={`/catalog/${viewpoint.path}`}
                element={
                  <CatalogView
                    key={viewpoint.path}
                    viewpointKinds={viewpoint.kinds}
                    viewpointPlugins={viewpoint.plugins}
                    viewpointColumns={viewpoint.columns}
                    heading={viewpoint.heading}
                    subheading={viewpoint.subheading}
                  />
                }
              />
            ))}
            <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
          </Routes>
        </Suspense>
      </Router>
    </QueryClientProvider>
  );
}
export default App;
