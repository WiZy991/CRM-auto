import { fileURLToPath, URL } from 'node:url';

import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],

  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },

  server: {
    port: 5173,
    // Слушаем все интерфейсы: страницу нужно открывать с телефона в той же
    // сети, чтобы проверять вёрстку на настоящем устройстве, а не в
    // эмуляции размеров окна.
    host: true,
    strictPort: true,

    proxy: {
      // Обращения к API идут через тот же источник, что и страница.
      // Так браузер не считает запросы межсайтовыми, и режим cookie
      // SameSite=Strict работает в разработке так же, как в бою.
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
      '/uploads': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },

  build: {
    target: 'es2022',
    sourcemap: true,
    // Исходники в карте не публикуем: она нужна для расшифровки трасс,
    // а выкладывать вместе с ней весь код приложения незачем.
  },

  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    exclude: ['**/node_modules/**', '**/dist/**', '**/e2e/**'],
  },
});
