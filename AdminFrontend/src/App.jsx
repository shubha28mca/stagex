import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./context/AuthContext";
import AdminLayout from "./layout/AdminLayout";
import LoginPage from "./pages/LoginPage";

import OpsDashboard from "./pages/ops/OpsDashboard";
import OpsEventsPage from "./pages/ops/OpsEventsPage";
import OpsParticipantsPage from "./pages/ops/OpsParticipantsPage";
import {
  EventTypesPage, AgeBandsPage, TaxonomyPage, CouponsPage, HallsPage, JudgesPage, SponsorsPage, CrewPoolPage, VendorsPage,
} from "./pages/ops/masters";

import EventDashboard from "./pages/event/EventDashboard";
import EventsPage from "./pages/event/EventsPage";
import EventParticipantsPage from "./pages/event/EventParticipantsPage";

// homeFor maps a role to its landing path — used for redirects.
const homeFor = (role) => (role === "ops" ? "/ops" : role === "event" ? "/event" : "/login");

// RequireRole gates a route tree to a single role. A logged-in admin of the
// wrong role is redirected to their own home, so the Ops and Event Admin areas
// never share a reachable path.
function RequireRole({ role, children }) {
  const { isAuthenticated, role: current } = useAuth();
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  if (current !== role) return <Navigate to={homeFor(current)} replace />;
  return children;
}

// Landing sends visitors to their role home or the login screen.
function Landing() {
  const { isAuthenticated, role } = useAuth();
  return <Navigate to={isAuthenticated ? homeFor(role) : "/login"} replace />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      {/* Operational Admin area (role: ops) */}
      <Route
        path="/ops/*"
        element={
          <RequireRole role="ops">
            <AdminLayout>
              <Routes>
                <Route index element={<OpsDashboard />} />
                <Route path="event-types" element={<EventTypesPage />} />
                <Route path="age-bands" element={<AgeBandsPage />} />
                <Route path="categories" element={<TaxonomyPage />} />
                <Route path="coupons" element={<CouponsPage />} />
                <Route path="halls" element={<HallsPage />} />
                <Route path="judges" element={<JudgesPage />} />
                <Route path="vendors" element={<VendorsPage />} />
                <Route path="sponsors" element={<SponsorsPage />} />
                <Route path="crew" element={<CrewPoolPage />} />
                <Route path="events" element={<OpsEventsPage />} />
                <Route path="participants" element={<OpsParticipantsPage />} />
                <Route path="*" element={<Navigate to="/ops" replace />} />
              </Routes>
            </AdminLayout>
          </RequireRole>
        }
      />

      {/* Event Admin area (role: event) */}
      <Route
        path="/event/*"
        element={
          <RequireRole role="event">
            <AdminLayout>
              <Routes>
                <Route index element={<EventDashboard />} />
                <Route path="events" element={<EventsPage />} />
                <Route path="participants" element={<EventParticipantsPage />} />
                <Route path="*" element={<Navigate to="/event" replace />} />
              </Routes>
            </AdminLayout>
          </RequireRole>
        }
      />

      <Route path="*" element={<Landing />} />
    </Routes>
  );
}
