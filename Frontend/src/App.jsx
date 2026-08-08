import { Navigate, Route, Routes } from "react-router-dom";
import Layout from "./layout/Layout";
import { useAuth } from "./context/AuthContext";
import AuthPage from "./pages/AuthPage";
import DiscoverPage from "./pages/DiscoverPage";
import EventDetailPage from "./pages/EventDetailPage";
import MyPeoplePage from "./pages/MyPeoplePage";
import MyEventsPage from "./pages/MyEventsPage";
import CertificatesPage from "./pages/CertificatesPage";

// RequireAuth guards the family-scoped screens: unauthenticated visitors are
// bounced to the auth page.
function RequireAuth({ children }) {
  const { isAuthenticated } = useAuth();
  return isAuthenticated ? children : <Navigate to="/auth" replace />;
}

// App defines the route table. Discover and event detail are public (browse
// before login); My* screens require a session.
export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Navigate to="/discover" replace />} />
        <Route path="/auth" element={<AuthPage />} />
        <Route path="/discover" element={<DiscoverPage />} />
        <Route path="/events/:id" element={<EventDetailPage />} />
        <Route
          path="/my/people"
          element={
            <RequireAuth>
              <MyPeoplePage />
            </RequireAuth>
          }
        />
        <Route
          path="/my/events"
          element={
            <RequireAuth>
              <MyEventsPage />
            </RequireAuth>
          }
        />
        <Route
          path="/my/certificates"
          element={
            <RequireAuth>
              <CertificatesPage />
            </RequireAuth>
          }
        />
        <Route path="*" element={<Navigate to="/discover" replace />} />
      </Routes>
    </Layout>
  );
}
