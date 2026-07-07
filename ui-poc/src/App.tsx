import { ThemeProvider } from './contexts/ThemeContext';
import Dashboard from './pages/Dashboard';
import { HashRouter as Router, Routes, Route } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogDetail from './pages/CatalogDetail';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/catalog" element={<CatalogView />} />
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
