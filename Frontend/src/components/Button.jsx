// Button — the one button primitive. Variants map to the StageX gradient CTAs.
// Reusable across projects: it only depends on theme.css classes.
export default function Button({ variant = "p", size, block, className = "", ...props }) {
  const classes = [
    "btn",
    `btn-${variant}`, // p (purple) | g (gold) | o (outline)
    size === "sm" ? "btn-sm" : "",
    block ? "btn-block" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");
  return <button className={classes} {...props} />;
}
