import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from './lib/queryClient';
import { ThemeProvider } from './contexts/ThemeContext';
import { BrowserRouter as Router, Routes, Route } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogDetail from './pages/CatalogDetail';
import PluginsPage from './pages/PluginsPage';
import { CATALOG_VIEWPOINTS } from './config/viewpoints';
import Overview from './pages/Overview';

function App() {
  return (
    <QueryClientProvider client={queryClient}>
    <ThemeProvider>
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
                  allowedKinds={viewpoint.allowedKinds}
                  allowedPlugins={viewpoint.allowedPlugins}
                  heading={viewpoint.heading}
                  subheading={viewpoint.subheading}
                />
              }
            />
          ))}
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
    </QueryClientProvider>
  );
}
export default App;
