// Panel — a titled white card. `title` and `actions` render in the header.
export default function Panel({ title, actions, children }) {
  return (
    <section className="panel">
      {(title || actions) && (
        <header className="ph">
          {title && <h3>{title}</h3>}
          {actions}
        </header>
      )}
      <div className="pb">{children}</div>
    </section>
  );
}
