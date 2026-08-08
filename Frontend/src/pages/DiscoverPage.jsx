import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { eventsApi } from "../api/endpoints";
import { EventCard, Field, Chip, Spinner, Alert, Empty, Pagination, usePaged } from "../components";

// DiscoverPage — search + filter events and show minimal-info cards
// (ClientDesignWeb §4). Filters are applied server-side via query params.
export default function DiscoverPage() {
  const navigate = useNavigate();
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [query, setQuery] = useState("");
  const [city, setCity] = useState("");
  const [maxFee, setMaxFee] = useState(0);
  const [rounds, setRounds] = useState(0);

  const { pageItems, page, setPage, totalPages } = usePaged(events, 9);

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const data = await eventsApi.list({ query, city, maxFee: maxFee || "", rounds: rounds || "" });
      setEvents(data || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  // Reload whenever a filter changes (debounced lightly by the effect batching).
  useEffect(() => {
    const t = setTimeout(load, 250);
    setPage(1);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, city, maxFee, rounds]);

  return (
    <>
      <div className="mb">
        <h1 style={{ color: "var(--navy)", fontSize: "1.45rem" }}>Discover events</h1>
        <p className="muted">Find championships and talent events for your family.</p>
      </div>

      <div className="grid2">
        <Field label="Search events" value={query} onChange={setQuery} />
        <Field label="City" value={city} onChange={setCity} />
      </div>

      <div className="row mb">
        <span className="muted" style={{ fontSize: ".8rem" }}>Fee:</span>
        <Chip active={maxFee === 0} onClick={() => setMaxFee(0)}>Any</Chip>
        <Chip active={maxFee === 300} onClick={() => setMaxFee(300)}>Under ₹300</Chip>
        <Chip active={maxFee === 500} onClick={() => setMaxFee(500)}>Under ₹500</Chip>
        <span className="muted" style={{ fontSize: ".8rem", marginLeft: 10 }}>Rounds:</span>
        <Chip active={rounds === 0} onClick={() => setRounds(0)}>Any</Chip>
        <Chip active={rounds === 2} onClick={() => setRounds(2)}>2+</Chip>
        <Chip active={rounds === 3} onClick={() => setRounds(3)}>3+</Chip>
      </div>

      <Alert>{error}</Alert>

      {loading ? (
        <Spinner />
      ) : events.length === 0 ? (
        <Empty>No events match — try clearing the filters.</Empty>
      ) : (
        <>
          <div className="ecards">
            {pageItems.map((e) => (
              <EventCard key={e.id} event={e} onOpen={(id) => navigate(`/events/${id}`)} />
            ))}
          </div>
          <Pagination page={page} totalPages={totalPages} onChange={setPage} />
        </>
      )}
    </>
  );
}
