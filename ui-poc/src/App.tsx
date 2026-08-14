import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from './lib/queryClient';
import { ThemeProvider } from './contexts/ThemeContext';
import { BrowserRouter as Router, Routes, Route } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogDetail from './pages/CatalogDetail';

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <Router>
          <Routes>
            <Route path="/" element={<CatalogView />} />
            <Route path="/catalog" element={<CatalogView />} />
            <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
          </Routes>
        </Router>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
export default App;
