import { NavLink, useNavigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";
import { initials } from "../components";

// Layout — the sticky StageX top bar plus the routed page content. Tabs only
// show when authenticated; the avatar toggles login/logout.
export default function Layout({ children }) {
  const { family, isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();

  const tabs = [
    { to: "/discover", label: "Discover" },
    { to: "/my/events", label: "My Events" },
    { to: "/my/people", label: "My People" },
    { to: "/my/certificates", label: "Certificates" },
  ];

  return (
    <>
      <header className="top">
        <div className="in">
          <NavLink to="/discover" className="logo">
            <span className="mark">✦</span>
            <span>
              StageX
              <small>Participant</small>
            </span>
          </NavLink>

          {isAuthenticated && (
            <nav className="tabs">
              {tabs.map((t) => (
                <NavLink
                  key={t.to}
                  to={t.to}
                  className={({ isActive }) => `tab ${isActive ? "on" : ""}`}
                >
                  {t.label}
                </NavLink>
              ))}
            </nav>
          )}

          {isAuthenticated ? (
            <button
              className="avatar"
              title={`${family.displayName} — click to log out`}
              onClick={() => {
                logout();
                navigate("/auth");
              }}
            >
              {initials(family.displayName) || "SX"}
            </button>
          ) : (
            <button className="avatar" title="Login" onClick={() => navigate("/auth")}>
              ⇥
            </button>
          )}
        </div>
      </header>

      <main className="wrap" style={{ padding: "24px 20px 60px" }}>
        {children}
      </main>
    </>
  );
}
