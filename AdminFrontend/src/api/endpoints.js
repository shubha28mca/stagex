// Typed admin API surface. A small `crud` factory removes boilerplate for the
// many Operational-Admin master resources that share the same REST shape.
import { request, uploadFile, downloadFile } from "./client";

const crud = (base) => ({
  list: () => request(base),
  create: (body) => request(base, { method: "POST", body }),
  update: (id, body) => request(`${base}/${id}`, { method: "PATCH", body }),
  remove: (id) => request(`${base}/${id}`, { method: "DELETE" }),
});
export const authApi = {
  login: (email, password) => request("/admin/auth/login", { method: "POST", body: { email, password } }),
  me: () => request("/admin/auth/me"),
};

// Operational Admin — master data + oversight + participants (role: ops).
export const opsApi = {
  dashboard: () => request("/admin/ops/dashboard"),
  eventTypes: crud("/admin/ops/event-types"),
  ageBands: crud("/admin/ops/age-bands"),
  categories: crud("/admin/ops/categories"),
  coupons: crud("/admin/ops/coupons"),
  halls: crud("/admin/ops/halls"),
  // Hall seating arrangements. A hall has at most one, so every call is keyed
  // by hallId rather than by a seat-map id.
  seatMaps: {
    index: () => request("/admin/ops/seat-maps"),
    get: (hallId) => request(`/admin/ops/halls/${hallId}/seat-map`),
    create: (hallId) => request(`/admin/ops/halls/${hallId}/seat-map`, { method: "POST" }),
    // Full replace of the arrangement — what the designer's Save sends.
    save: (hallId, body) => request(`/admin/ops/halls/${hallId}/seat-map`, { method: "PUT", body }),
    remove: (hallId) => request(`/admin/ops/halls/${hallId}/seat-map`, { method: "DELETE" }),
    publish: (hallId) => request(`/admin/ops/halls/${hallId}/seat-map/publish`, { method: "POST" }),
    unpublish: (hallId) => request(`/admin/ops/halls/${hallId}/seat-map/unpublish`, { method: "POST" }),
    // Assign a tier / price / blocked flag to already-saved seats.
    updateSeats: (hallId, body) =>
      request(`/admin/ops/halls/${hallId}/seat-map/seats`, { method: "PATCH", body }),
    // Lay out a block of rows server-side (no save) so the editor and the
    // stored geometry can never drift apart.
    generateRows: (spec) => request("/admin/ops/seat-maps/generate-rows", { method: "POST", body: spec }),
  },
  judges: crud("/admin/ops/judges"),
  sponsors: crud("/admin/ops/sponsors"),
  crew: crud("/admin/ops/crew"),
  vendors: crud("/admin/ops/vendors"),
  events: {
    list: () => request("/admin/ops/events"),
    update: (id, body) => request(`/admin/ops/events/${id}`, { method: "PATCH", body }),
    remove: (id) => request(`/admin/ops/events/${id}`, { method: "DELETE" }),
  },
  // Per-event Ops operations.
  eventCrew: {
    list: (eventId) => request(`/admin/ops/events/${eventId}/crew`),
    assign: (eventId, crewId) => request(`/admin/ops/events/${eventId}/crew/assign`, { method: "POST", body: { crewId } }),
    remove: (assignmentId) => request(`/admin/ops/event-crew/${assignmentId}`, { method: "DELETE" }),
  },
  eventVendors: {
    list: (eventId) => request(`/admin/ops/events/${eventId}/vendors`),
    assign: (eventId, vendorId, cost) => request(`/admin/ops/events/${eventId}/vendors/assign`, { method: "POST", body: { vendorId, cost } }),
    remove: (id) => request(`/admin/ops/event-vendors/${id}`, { method: "DELETE" }),
  },
  eventSponsors: {
    list: (eventId) => request(`/admin/ops/events/${eventId}/sponsors`),
    assign: (eventId, sponsorId, cost) => request(`/admin/ops/events/${eventId}/sponsors/assign`, { method: "POST", body: { sponsorId, cost } }),
    remove: (id) => request(`/admin/ops/event-sponsors/${id}`, { method: "DELETE" }),
  },
  expenses: {
    list: (eventId) => request(`/admin/ops/events/${eventId}/expenses`),
    add: (eventId, body) => request(`/admin/ops/events/${eventId}/expenses`, { method: "POST", body }),
    remove: (id) => request(`/admin/ops/expenses/${id}`, { method: "DELETE" }),
  },
  pnl: (eventId) => request(`/admin/ops/events/${eventId}/pnl`),
  // Ticket sales oversight — read-mostly. The one write is settling an
  // offline refund, which is deliberately an admin-only action because money
  // is never returned automatically.
  ticketStats: () => request("/admin/ops/ticket-stats"),
  // Seat-by-seat occupancy for one event: what is available, paid for, being
  // paid for, or blocked — plus the plan geometry so it can be drawn.
  seatOccupancy: (eventId) => request(`/admin/ops/events/${eventId}/seat-occupancy`),
  // The seat-reservation report, generated on demand. CSV lists every seat;
  // PDF is the printable summary with the event P&L.
  seatReport: (eventId, format = "csv") =>
    downloadFile(
      `/admin/ops/events/${eventId}/seat-report?format=${format}`,
      `stagex-seat-reservation-report.${format}`
    ),
  bookings: (eventId = "", status = "") => {
    const q = new URLSearchParams();
    if (eventId) q.set("eventId", eventId);
    if (status) q.set("status", status);
    const qs = q.toString();
    return request(`/admin/ops/bookings${qs ? `?${qs}` : ""}`);
  },
  settleRefund: (bookingId, refundStatus, note = "") =>
    request(`/admin/ops/bookings/${bookingId}/refund`, {
      method: "PATCH",
      body: { refundStatus, note },
    }),
  participants: {
    list: () => request("/admin/ops/participants"),
    update: (id, body) => request(`/admin/ops/participants/${id}`, { method: "PATCH", body }),
    remove: (id) => request(`/admin/ops/participants/${id}`, { method: "DELETE" }),
  },
};

// Event Admin — own events + categories + participants (role: event).
export const eventApi = {
  dashboard: () => request("/admin/event/dashboard"),
  events: {
    list: () => request("/admin/event/events"),
    create: (body) => request("/admin/event/events", { method: "POST", body }),
    update: (id, body) => request(`/admin/event/events/${id}`, { method: "PATCH", body }),
    remove: (id) => request(`/admin/event/events/${id}`, { method: "DELETE" }),
    publish: (id) => request(`/admin/event/events/${id}/publish`, { method: "POST" }),
  },
  eventCategories: {
    list: (eventId) => request(`/admin/event/events/${eventId}/categories`),
    add: (eventId, body) => request(`/admin/event/events/${eventId}/categories`, { method: "POST", body }),
    remove: (ecId) => request(`/admin/event/categories/${ecId}`, { method: "DELETE" }),
  },
  ref: {
    categories: () => request("/admin/event/ref/categories"),
    ageBands: () => request("/admin/event/ref/age-bands"),
    judges: () => request("/admin/event/ref/judges"),
  },
  participants: {
    list: () => request("/admin/event/participants"),
    update: (id, body) => request(`/admin/event/participants/${id}`, { method: "PATCH", body }),
  },
  // Ad-hoc participant added by the admin, paid offline.
  addOffline: (eventId, body) =>
    request(`/admin/event/events/${eventId}/participants/offline`, { method: "POST", body }),
  crew: {
    list: (eventId) => request(`/admin/event/events/${eventId}/crew`),
    add: (eventId, body) => request(`/admin/event/events/${eventId}/crew`, { method: "POST", body }),
    remove: (crewId) => request(`/admin/event/crew/${crewId}`, { method: "DELETE" }),
  },
  notifications: {
    getConfig: (eventId) => request(`/admin/event/events/${eventId}/notifications/config`),
    setConfig: (eventId, config) => request(`/admin/event/events/${eventId}/notifications/config`, { method: "PUT", body: config }),
    list: (eventId) => request(`/admin/event/events/${eventId}/notifications`),
    send: (eventId, body) => request(`/admin/event/events/${eventId}/notifications`, { method: "POST", body }),
  },
  certificates: {
    list: (eventId) => request(`/admin/event/events/${eventId}/certificates`),
    issue: (eventId, body) => request(`/admin/event/events/${eventId}/certificates`, { method: "POST", body }),
    remove: (certId) => request(`/admin/event/certificates/${certId}`, { method: "DELETE" }),
  },
  media: {
    list: (eventId) => request(`/admin/event/events/${eventId}/media`),
    upload: (eventId, file, kind) => uploadFile(`/admin/event/events/${eventId}/media`, file, { kind }),
    remove: (mediaId) => request(`/admin/event/media/${mediaId}`, { method: "DELETE" }),
  },
};
