import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: { host: "127.0.0.1", port: 9245, strictPort: true },
  build: {
    target: "es2022",
    chunkSizeWarningLimit: 900,
    rollupOptions: { input: { main: path.resolve(__dirname, "index.html"), installer: path.resolve(__dirname, "installer.html") } },
  },
});
