import { ThemeProvider } from './contexts/ThemeContext';
import { BrowserRouter as Router, Routes, Route, Navigate } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogDetail from './pages/CatalogDetail';
import { CATALOG_VIEWPOINTS } from './config/viewpoints';
import Overview from './pages/Overview';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/catalog" element={<Overview />} />
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
  );
}
export default App;
