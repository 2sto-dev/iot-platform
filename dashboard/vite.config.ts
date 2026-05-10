import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const kongUrl = env.VITE_KONG_URL ?? "http://localhost:8000";
  const goUrl = env.VITE_GO_URL ?? "http://172.16.0.105:8090";

  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: 5173,
      proxy: {
        "/api": { target: kongUrl, changeOrigin: true },
        // /go merge direct la Go — ruta Kong /go dă 500 (pre-function Lua plugin issue);
        // Go re-validează JWT-ul singur, deci direct e safe în dev.
        "/go":  { target: goUrl, changeOrigin: true },
      },
    },
  };
});
