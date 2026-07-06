import { ThemeProvider } from './contexts/ThemeContext';
import Dashboard from './pages/Dashboard';
import { HashRouter as Router, Routes, Route } from "react-router";
import ModelRegistries from './pages/ModelRegistries';
import ModelSpec from './pages/ModelSpec';
import CatalogOverview from './pages/CatalogOverview';
import CatalogList from './pages/CatalogList';
import CatalogDetail from './pages/CatalogDetail';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/model-registry" element={<ModelRegistries />} />
          <Route path="/model-registry/:id" element={<ModelSpec />} />
          <Route path="/catalog" element={<CatalogOverview />} />
          <Route path="/catalog/:kind" element={<CatalogList />} />
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
