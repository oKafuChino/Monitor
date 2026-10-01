import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  root: path.resolve(import.meta.dirname, "src/share"),
  publicDir: false,
  plugins: [react()],
  build: {
    outDir: path.resolve(import.meta.dirname, "dist-share"),
    emptyOutDir: true,
    sourcemap: false,
  },
});
