// Typed API surface. Grouping the endpoints here keeps components free of URL
// strings and makes the backend contract easy to see at a glance. Each function
// maps 1:1 to an operation in Backend/docs/openapi.yaml.
import { request } from "./client";

export const authApi = {
  sendOtp: (phone, purpose) =>
    request("/api/auth/otp/send", { method: "POST", body: { phone, purpose } }),
  verifyOtp: (phone, code, purpose) =>
    request("/api/auth/otp/verify", { method: "POST", body: { phone, code, purpose } }),
  register: (phone, name, password, otp) =>
    request("/api/auth/register", { method: "POST", body: { phone, name, password, otp } }),
  login: (phone, { password, otp }) =>
    request("/api/auth/login", { method: "POST", body: { phone, password, otp } }),
};

export const catalogApi = {
  eventTypes: () => request("/api/catalog/event-types"),
  categories: () => request("/api/catalog/categories"),
  ageBands: () => request("/api/catalog/age-bands"),
};

export const eventsApi = {
  list: (filters) => request("/api/events", { query: filters }),
  get: (id) => request(`/api/events/${id}`),
};

export const couponsApi = {
  validate: (code, subtotal, eventId) =>
    request("/api/coupons/validate", { method: "POST", body: { code, subtotal, eventId } }),
};

export const peopleApi = {
  list: () => request("/api/people"),
  create: (person) => request("/api/people", { method: "POST", body: person }),
  update: (id, patch) => request(`/api/people/${id}`, { method: "PATCH", body: patch }),
  remove: (id) => request(`/api/people/${id}`, { method: "DELETE" }),
};

export const registrationsApi = {
  create: (payload) => request("/api/registrations", { method: "POST", body: payload }),
};

export const paymentsApi = {
  createOrder: (registrationId) =>
    request("/api/payments/order", { method: "POST", body: { registrationId } }),
  confirm: (registrationId, success) =>
    request("/api/payments/confirm", { method: "POST", body: { registrationId, success } }),
};

export const myApi = {
  events: () => request("/api/my/events"),
  certificates: () => request("/api/my/certificates"),
  notifications: () => request("/api/my/notifications"),
};

export const feedbackApi = {
  submit: (payload) => request("/api/feedback", { method: "POST", body: payload }),
};
