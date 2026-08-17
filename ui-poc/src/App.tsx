import { ThemeProvider } from './contexts/ThemeContext';
import { BrowserRouter as Router, Routes, Route, Navigate } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogKindView from './pages/CatalogKindView';
import CatalogDetail from './pages/CatalogDetail';
import { CATALOG_VIEWPOINTS } from './config/viewpoints';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<CatalogView />} />
          <Route path="/catalog" element={<CatalogView />} />
          {CATALOG_VIEWPOINTS.map((viewpoint) => (
            <Route
              key={viewpoint.path}
              path={`/catalog/${viewpoint.path}`}
              element={
                <CatalogView
                  allowedKinds={viewpoint.allowedKinds}
                  allowedPlugins={viewpoint.allowedPlugins}
                  heading={viewpoint.heading}
                  subheading={viewpoint.subheading}
                />
              }
            />
          ))}
          <Route path="/catalog/kinds/:kind" element={<CatalogKindView />} />
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
