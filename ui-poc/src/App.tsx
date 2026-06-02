import { ThemeProvider } from './contexts/ThemeContext';
import Dashboard from './pages/Dashboard';
import { HashRouter as Router, Routes, Route } from "react-router";
import ModelRegistries from './pages/ModelRegistries';
import DatasetRegistries from './pages/DatasetRegistries';
import CatalogGraph from './pages/CatalogGraph';
import ModelSpec from './pages/ModelSpec';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/model-registry" element={<ModelRegistries />} />
          <Route path="/dataset-registry" element={<DatasetRegistries />} />
          <Route path="/catalog-graph" element={<CatalogGraph />} />
          <Route path="/model-registry/:id" element={<ModelSpec />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
