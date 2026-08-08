// =============================================================================
// SINGLE SOURCE OF TRUTH for the Admin backend location.
//
// Change this one value (or the VITE_ADMIN_API_BASE_URL build arg) to re-target
// the admin API when hosting. Every request flows through src/api/client.js.
// =============================================================================

export const ADMIN_API_BASE_URL =
  import.meta.env.VITE_ADMIN_API_BASE_URL?.replace(/\/$/, "") || "http://localhost:8081";

export const TOKEN_STORAGE_KEY = "stagex.admin.token";
export const ADMIN_STORAGE_KEY = "stagex.admin.user";
