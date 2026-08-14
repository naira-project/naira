import { ThemeProvider } from './contexts/ThemeContext';
import { BrowserRouter as Router, Routes, Route, Navigate } from "react-router";
import CatalogView from './pages/CatalogView';
import CatalogKindView from './pages/CatalogKindView';
import CatalogDetail from './pages/CatalogDetail';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/" element={<CatalogView />} />
          <Route path="/catalog" element={<CatalogView />} />
          <Route
            path="/catalog/software_catalog"
            element={
              <CatalogView
                allowedKinds={["deployment", "service"]}
                heading="Software Catalog"
                subheading="Deployments and services running in the cluster."                
                />
            }
          />
          <Route 
            path="/catalog/model"
            element={
              <CatalogView 
                allowedKinds={["model"]}
                heading="Model"
                subheading="Models registered in the catalog."
              />
            }
          />
          <Route path="/catalog/kinds/:kind" element={<CatalogKindView />} />
          <Route path="/catalog/:kind/*" element={<CatalogDetail />} />
        </Routes>
    </Router>
    </ThemeProvider>
  );
}
export default App;
