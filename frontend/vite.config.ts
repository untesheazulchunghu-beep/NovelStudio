import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'NOVELSTUDIO_')

  return {
    plugins: [react()],
    server: {
      port: 5173,
      host: 'localhost',
      proxy: {
        '/api': env.NOVELSTUDIO_API_TARGET ?? 'http://127.0.0.1:8080',
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
  }
})
