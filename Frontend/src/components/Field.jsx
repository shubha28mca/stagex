// Field — a floating-label input/select/textarea. One component covers all three
// so forms across the app (and other projects) stay visually consistent.
//
// Note: the input needs placeholder=" " for the CSS floating-label trick.
export default function Field({
  as = "input",
  label,
  value,
  onChange,
  error,
  hint,
  options, // for select: [{value,label}]
  ...props
}) {
  const handle = (e) => onChange?.(e.target.value);
  return (
    <div className="f">
      {as === "select" ? (
        <select
          className={value ? "filled" : ""}
          value={value}
          onChange={handle}
          {...props}
        >
          <option value="" disabled hidden></option>
          {options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : as === "textarea" ? (
        <textarea placeholder=" " value={value} onChange={handle} {...props} />
      ) : (
        <input placeholder=" " value={value} onChange={handle} {...props} />
      )}
      <label>{label}</label>
      {error ? (
        <div className="fmsg e">⚠ {error}</div>
      ) : hint ? (
        <div className="fmsg h">{hint}</div>
      ) : null}
    </div>
  );
}
