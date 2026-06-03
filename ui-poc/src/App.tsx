import { ThemeProvider } from './contexts/ThemeContext';
import Dashboard from './pages/Dashboard';
import { HashRouter as Router, Routes, Route } from "react-router";
import ModelRegistries from './pages/ModelRegistries';
import ModelSpec from './pages/ModelSpec';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/model-registry" element={<ModelRegistries />} />
          <Route path="/model-registry/:id" element={<ModelSpec />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
