import { useEffect, useState } from "react";
import { opsApi } from "../../api/endpoints";
import { Spinner, Alert } from "../../components/ui";

// OpsDashboard shows the platform-wide KPIs (Admin Design §4.10, condensed).
export default function OpsDashboard() {
  const [d, setD] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    opsApi.dashboard().then(setD).catch((e) => setError(e.message));
  }, []);

  if (error) return <Alert>{error}</Alert>;
  if (!d) return <Spinner />;

  const kpis = [
    { l: "Events", n: d.events },
    { l: "Registrations", n: d.registrations },
    { l: "Participants", n: d.participants },
    { l: "Revenue", n: `₹${Number(d.revenue).toLocaleString()}` },
  ];
  return (
    <>
      <div className="ptitle">
        <div>
          <h1>Operations Dashboard</h1>
          <p>Platform-wide health across every Event Admin.</p>
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
