import path from "path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "../internal/web/spa/dist",
    emptyOutDir: true,
    // CorticalStack is a local-only app served by the Go binary from
    // localhost, so the vite chunk-size warning (aimed at over-the-wire
    // production apps) is noise here. Route-based code splitting would
    // trade ~180 KB of first-load parse time for a visible loading
    // flash on first visit to each page — not a good tradeoff for a
    // localhost UX. Revisit if the server is ever exposed via a tunnel
    // or proxy. See docs/code-review-ui.md "Bundle code splitting".
    chunkSizeWarningLimit: 1000,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:8000",
        changeOrigin: true,
      },
    },
  },
})
