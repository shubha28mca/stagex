// The admin HTTP client. Every network call goes through `request`, centralizing
// the base URL, bearer token, {data}/{error} envelope and error handling.
import { ADMIN_API_BASE_URL, TOKEN_STORAGE_KEY } from "../config";

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

export async function request(path, { method = "GET", body } = {}) {
  const res = await fetch(`${ADMIN_API_BASE_URL}${path}`, {
    method,
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const payload = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = payload.error || {};
    throw new ApiError(err.message || "Request failed", err.code, res.status);
  }
  return payload.data;
}

// downloadFile fetches a binary endpoint with the auth header and saves it. Used
// for report exports (CSV/PDF) and the archive (POST), which are not JSON.
export async function downloadFile(path, filename, method = "GET") {
  const res = await fetch(`${ADMIN_API_BASE_URL}${path}`, { method, headers: { ...authHeader() } });
  if (!res.ok) throw new ApiError("Download failed", "download", res.status);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// uploadFile POSTs a multipart form (file + fields) with the auth header. The
// browser sets the multipart boundary, so we must not set Content-Type.
export async function uploadFile(path, file, fields = {}) {
  const fd = new FormData();
  fd.append("file", file);
  Object.entries(fields).forEach(([k, v]) => fd.append(k, v));
  const res = await fetch(`${ADMIN_API_BASE_URL}${path}`, {
    method: "POST",
    headers: { ...authHeader() },
    body: fd,
  });
  const payload = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = payload.error || {};
    throw new ApiError(err.message || "Upload failed", err.code, res.status);
  }
  return payload.data;
}
