// The HTTP client. Every network call in the app goes through `request`, so
// concerns like the base URL, the auth header, the {data}/{error} envelope and
// error handling live in exactly one place.
import { API_BASE_URL, TOKEN_STORAGE_KEY } from "../config";

/**
 * ApiError carries the backend's stable error code and message so UI components
 * can show a friendly message and, if needed, branch on the code.
 */
export class ApiError extends Error {
  constructor(message, code, status) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

function authHeader() {
  const token = localStorage.getItem(TOKEN_STORAGE_KEY);
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/**
 * request performs a JSON API call and unwraps the {data} envelope.
 * @param {string} path   e.g. "/api/events"
 * @param {object} [opts] { method, body, query }
 * @returns the `data` field of the response
 */
export async function request(path, { method = "GET", body, query } = {}) {
  let url = `${API_BASE_URL}${path}`;
  if (query) {
    const params = new URLSearchParams(
      Object.entries(query).filter(([, v]) => v !== "" && v != null)
    );
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }

  const res = await fetch(url, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...authHeader(),
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  // 204 No Content (e.g. CORS preflight passthrough) → nothing to parse.
  if (res.status === 204) return null;

  const payload = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = payload.error || {};
    throw new ApiError(err.message || "Request failed", err.code, res.status);
  }
  return payload.data;
}
