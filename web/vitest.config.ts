import { defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  test: {
    // The unit tests live in `test/` next to the sources they cover.
    include: ['test/**/*.{test,spec}.{js,mjs,ts,mts,jsx,tsx}', 'src/**/*.{test,spec}.{js,mjs,ts,mts,jsx,tsx}'],
    // Files suffixed `.node.test.mjs` use the Node.js built-in test
    // runner (`node --test`) instead of vitest — they must not be
    // collected here.
    exclude: ['**/node_modules/**', '**/dist/**', 'test/**/*.node.test.mjs'],
    environment: 'node',
    globals: false,
    reporters: 'default',
  },
})