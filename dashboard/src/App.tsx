import { Routes, Route, Navigate } from "react-router-dom";
import { isAuthenticated } from "./lib/auth";
import LoginPage from "./pages/LoginPage";
import Layout from "./components/Layout";
import DevicesPage from "./pages/DevicesPage";
import SolarPage from "./pages/SolarPage";
import BoilerPage from "./pages/BoilerPage";
import RulesPage from "./pages/RulesPage";
import NotificationsPage from "./pages/NotificationsPage";
import AuditPage from "./pages/AuditPage";

function RequireAuth({ children }: { children: React.ReactNode }) {
  return isAuthenticated() ? <>{children}</> : <Navigate to="/login" replace />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/devices" replace />} />
        <Route path="devices" element={<DevicesPage />} />
        <Route path="solar" element={<SolarPage />} />
        <Route path="boiler" element={<BoilerPage />} />
        <Route path="rules" element={<RulesPage />} />
        <Route path="notifications" element={<NotificationsPage />} />
        <Route path="audit" element={<AuditPage />} />
      </Route>
    </Routes>
  );
}
