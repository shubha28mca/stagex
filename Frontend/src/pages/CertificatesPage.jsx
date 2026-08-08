import { useEffect, useState } from "react";
import { myApi } from "../api/endpoints";
import { Panel, Badge, Spinner, Alert, Empty, Pagination, usePaged } from "../components";

const positionTone = { gold: "gold", silver: "purple", bronze: "pending", participation: "done" };

// CertificatesPage — every certificate earned across the family, with a
// download link (ClientDesignWeb §8).
export default function CertificatesPage() {
  const [certs, setCerts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const { pageItems, page, setPage, totalPages } = usePaged(certs, 6);

  useEffect(() => {
    (async () => {
      try {
        setCerts((await myApi.certificates()) || []);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  if (loading) return <Spinner />;

  return (
    <>
      <div className="mb">
        <h1 style={{ color: "var(--navy)", fontSize: "1.45rem" }}>Certificates</h1>
        <p className="muted">Wins and participation across the family.</p>
      </div>
      <Alert>{error}</Alert>

      {certs.length === 0 ? (
        <Empty>No certificates yet — they appear here after events complete.</Empty>
      ) : (
        <>
          {pageItems.map((c) => (
            <Panel
              key={c.id}
              title={`${c.personName} — ${c.eventName}`}
              actions={<Badge tone={positionTone[c.position] || "purple"}>{c.position}</Badge>}
            >
              <div className="row spread">
                <div className="muted" style={{ fontSize: ".82rem" }}>
                  {c.categoryName} · Cert #{c.certCode} ·{" "}
                  {new Date(c.issuedAt).toLocaleDateString()}
                </div>
                {c.fileUrl && (
                  <a className="btn btn-o btn-sm" href={c.fileUrl} target="_blank" rel="noreferrer">
                    🔍 View
                  </a>
                )}
              </div>
              {c.fileUrl && (
                <img
                  src={c.fileUrl}
                  alt={`Certificate for ${c.personName}`}
                  style={{ marginTop: 10, maxWidth: "100%", maxHeight: 260, borderRadius: 12, border: "1px solid var(--line)" }}
                />
              )}
            </Panel>
          ))}
          <Pagination page={page} totalPages={totalPages} onChange={setPage} />
        </>
      )}
    </>
  );
}
