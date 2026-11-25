import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: '/arca/',
  plugins: [vue()],
  build: { outDir: '../web/dist/arca', emptyOutDir: true },
  server: {
    proxy: { '/api': 'http://localhost:3000', '/v1': 'http://localhost:3000' },
  },
})
