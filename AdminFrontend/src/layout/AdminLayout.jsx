import { NavLink, useNavigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";
import { initials } from "../components/ui";

// Nav definitions per role. The two lists share no path — Ops lives under /ops
// and Event Admin under /event — so the two consoles never overlap.
const OPS_NAV = [
  { group: "Overview", items: [{ to: "/ops", label: "Dashboard", end: true }] },
  {
    group: "Master data",
    items: [
      { to: "/ops/event-types", label: "Event Types" },
      { to: "/ops/age-bands", label: "Age Bands" },
      { to: "/ops/categories", label: "Taxonomy" },
      { to: "/ops/coupons", label: "Coupons" },
      { to: "/ops/halls", label: "Halls" },
      { to: "/ops/judges", label: "Judges" },
      { to: "/ops/vendors", label: "Vendors" },
      { to: "/ops/sponsors", label: "Sponsors" },
      { to: "/ops/crew", label: "Crew" },
    ],
  },
  {
    group: "Platform",
    items: [
      { to: "/ops/events", label: "All Events" },
      { to: "/ops/participants", label: "Participants" },
    ],
  },
];

const EVENT_NAV = [
  { group: "Overview", items: [{ to: "/event", label: "Dashboard", end: true }] },
  {
    group: "My work",
    items: [
      { to: "/event/events", label: "My Events" },
      { to: "/event/participants", label: "Participants" },
    ],
  },
];

// AdminLayout renders the top bar + navy sidebar shell around routed pages.
export default function AdminLayout({ children }) {
  const { admin, role, logout } = useAuth();
  const navigate = useNavigate();
  const nav = role === "ops" ? OPS_NAV : EVENT_NAV;
  const roleLabel = role === "ops" ? "Operational Admin" : "Event Admin";

  return (
    <>
      <header className="top">
        <div className="logo">
          <span className="mark">✦</span>
          <span>
            StageX<small>Admin Console</small>
          </span>
        </div>
        <span className="rolepill">{roleLabel}</span>
        <button className="logout" style={{ marginLeft: "auto" }} onClick={() => { logout(); navigate("/login"); }}>
          Log out
        </button>
        <span className="avatar">{initials(admin?.name) || "AD"}</span>
      </header>

      <div className="app">
        <aside>
          <div className="rolecard">
            <b>{admin?.name}</b>
            <small>{admin?.email}</small>
          </div>
          {nav.map((section) => (
            <div key={section.group}>
              <div className="ngroup">{section.group}</div>
              {section.items.map((it) => (
                <NavLink
                  key={it.to}
                  to={it.to}
                  end={it.end}
                  className={({ isActive }) => `nitem ${isActive ? "on" : ""}`}
                >
                  {it.label}
                </NavLink>
              ))}
            </div>
          ))}
        </aside>
        <main>{children}</main>
      </div>
    </>
  );
}
