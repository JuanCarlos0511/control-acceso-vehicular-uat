import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

// Cada app se compila por separado (--mode caseta | --mode admin) en un solo HTML,
// que es lo que Apps Script puede servir con HtmlService.
const APPS = { caseta: 'app-caseta', admin: 'portal-admin' } as const;

const ruta = (relativa: string) => fileURLToPath(new URL(relativa, import.meta.url));

export default defineConfig(({ mode }) => {
  const carpeta = APPS[mode as keyof typeof APPS];
  if (!carpeta) {
    throw new Error(`Indica la app con --mode caseta o --mode admin (recibido: "${mode}").`);
  }

  return {
    root: ruta(`./${carpeta}`),
    envDir: ruta('.'),
    resolve: { alias: { '@shared': ruta('./shared') } },
    plugins: [react(), tailwindcss(), viteSingleFile({ outputFileName: `${carpeta}.html` })],
    build: { outDir: ruta('./dist'), emptyOutDir: false },
    server: { port: mode === 'caseta' ? 5173 : 5174 },
  };
});
