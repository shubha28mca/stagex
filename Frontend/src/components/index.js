// Barrel export for the reusable component library. Importing from
// "../components" keeps page imports short and the library easy to lift into
// another project wholesale.
export { default as Button } from "./Button";
export { default as Badge } from "./Badge";
export { default as Field } from "./Field";
export { default as Panel } from "./Panel";
export { default as Chip } from "./Chip";
export { default as Stepper } from "./Stepper";
export { default as StarRating } from "./StarRating";
export { default as EventCard } from "./EventCard";
export { Spinner, Alert, Empty } from "./Feedback";
export { default as Pagination, usePaged } from "./Pagination";

// initials — shared helper for avatar bubbles.
export function initials(name = "") {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join("");
}
