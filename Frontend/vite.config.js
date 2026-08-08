import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Vite config. The dev server runs on 5173 (matches the backend CORS default).
// The API base URL is NOT hard-coded here — it is read at runtime from a single
// place (src/config/index.js) so re-targeting the backend is a one-line change.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
  },
});
