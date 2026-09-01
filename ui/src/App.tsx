import { QueryClientProvider } from '@tanstack/react-query';
import { Route, BrowserRouter as Router, Routes } from 'react-router';
import { CATALOG_VIEWPOINTS } from './config/viewpoints';
import { queryClient } from './lib/queryClient';
import CatalogDetail from './pages/CatalogDetail';
import CatalogView from './pages/CatalogView';
import Overview from './pages/Overview';
import PluginsPage from './pages/PluginsPage';

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Router>
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
      </Router>
    </QueryClientProvider>
  );
}
export default App;
