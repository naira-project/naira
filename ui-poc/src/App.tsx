import { ThemeProvider } from './contexts/ThemeContext';
import { HashRouter as Router, Routes, Route, Navigate } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogDetail from './pages/CatalogDetail';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<CatalogView />} />
          <Route path="/catalog" element={<CatalogView />} />
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
