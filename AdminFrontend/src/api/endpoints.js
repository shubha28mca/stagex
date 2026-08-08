// Typed admin API surface. A small `crud` factory removes boilerplate for the
// many Operational-Admin master resources that share the same REST shape.
import { request, uploadFile } from "./client";

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
