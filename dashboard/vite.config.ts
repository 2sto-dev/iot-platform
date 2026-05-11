import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const kongUrl = env.VITE_KONG_URL ?? "http://localhost:8000";

  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: 5173,
      proxy: {
        "/api": { target: kongUrl, changeOrigin: true },
        "/go":  { target: kongUrl, changeOrigin: true },
      },
    },
  };
});
