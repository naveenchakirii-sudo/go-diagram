import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// In development, run the Go server with `-addr 127.0.0.1:8080` and
// `npm run dev`; Vite proxies the websocket to it.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:8080', ws: true },
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
  test: { environment: 'node' },
});
