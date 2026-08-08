import { useEffect, useState } from "react";
import { eventApi } from "../../api/endpoints";
import { Spinner, Alert } from "../../components/ui";

// EventDashboard shows the Event Admin's own-events summary.
export default function EventDashboard() {
  const [d, setD] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    eventApi.dashboard().then(setD).catch((e) => setError(e.message));
  }, []);

  if (error) return <Alert>{error}</Alert>;
  if (!d) return <Spinner />;

  const kpis = [
    { l: "My events", n: d.events },
    { l: "Published", n: d.published },
    { l: "Registrations", n: d.registrations },
    { l: "Revenue", n: `₹${Number(d.revenue).toLocaleString()}` },
  ];
  return (
    <>
      <div className="ptitle">
        <div>
          <h1>Event Admin Dashboard</h1>
          <p>Only the events you created — plus what they’ve earned.</p>
        </div>
      </div>
      <div className="kpis">
        {kpis.map((k) => (
          <div className="kpi" key={k.l}>
            <div className="n">{k.n}</div>
            <div className="l">{k.l}</div>
          </div>
        ))}
      </div>
    </>
  );
}
