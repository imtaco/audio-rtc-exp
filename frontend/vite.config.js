import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';
import { readdirSync } from 'fs';

// Auto-discover HTML files in the root directory
const htmlFiles = readdirSync(resolve(__dirname)).filter(file => file.endsWith('.html'));
const input = Object.fromEntries(
  htmlFiles.map(file => [file.replace('.html', ''), resolve(__dirname, file)]),
);

export default defineConfig({
  plugins: [svelte()],
  server: {
    host: '0.0.0.0',
    port: 5173,
    cors: true,
  },
  build: {
    outDir: 'dist',
    rollupOptions: {
      input,
    },
  },
});
