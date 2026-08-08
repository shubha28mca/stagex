import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Admin console runs on its own port (5174) so it is completely separate from
// the participant web app (5173). The admin API base URL is read at runtime
// from src/config/index.js — one place to re-target the backend.
export default defineConfig({
  plugins: [react()],
  server: { port: 5174, host: true },
});
