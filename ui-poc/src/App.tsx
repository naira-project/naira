import { ThemeProvider } from './contexts/ThemeContext';
import Dashboard from './pages/Dashboard';
import { HashRouter as Router, Routes, Route } from "react-router";
import ModelRegistries from './pages/ModelRegistries';
import ModelSpec from './pages/ModelSpec';
import Datasets from './pages/Datasets';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/model-registry" element={<ModelRegistries />} />
          <Route path="/model-registry/:id" element={<ModelSpec />} />
          <Route path="/dataset-registry" element={<Datasets />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
