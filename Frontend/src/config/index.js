// =============================================================================
// SINGLE SOURCE OF TRUTH for the backend location.
//
// This is the one place to change when you host the backend somewhere new. In
// development it defaults to localhost; in a build it reads VITE_API_BASE_URL
// (set in .env or the Docker build arg). Nothing else in the app hard-codes a
// URL — every request goes through src/api/client.js, which reads API_BASE_URL.
// =============================================================================

export const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, "") || "http://localhost:8080";

// Where the JWT is persisted in the browser.
export const TOKEN_STORAGE_KEY = "stagex.token";
export const FAMILY_STORAGE_KEY = "stagex.family";
