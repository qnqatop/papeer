import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { cpSync, createReadStream, existsSync, statSync } from 'node:fs'
import { dirname, join, normalize, resolve, sep } from 'node:path'
import { createRequire } from 'node:module'

// pdf.js loads CMaps, standard fonts, ICC profiles and wasm decoders at
// runtime by URL. Serve them from our own origin in dev and copy them into the
// build output under /pdfjs/ so the viewer works offline and under a strict
// same-origin CSP (no CDN fetches).
const PDFJS_ASSET_DIRS = ['cmaps', 'standard_fonts', 'iccs', 'wasm']
const PDFJS_PUBLIC_PREFIX = '/pdfjs/'
// quickjs is only used by the PDF JavaScript sandbox, which we never enable.
const PDFJS_SKIP = /quickjs/

function pdfjsAssets(): Plugin {
  const pkgDir = dirname(createRequire(import.meta.url).resolve('pdfjs-dist/package.json'))
  let outDir: string | null = null
  return {
    name: 'papeer-pdfjs-assets',
    configResolved(config) {
      // Vitest also runs build hooks (with a dummy outDir); only copy on a real build.
      if (config.command === 'build' && !process.env.VITEST) outDir = resolve(config.root, config.build.outDir)
    },
    configureServer(server) {
      server.middlewares.use(PDFJS_PUBLIC_PREFIX, (req, res, next) => {
        const rel = normalize(decodeURIComponent((req.url ?? '').split('?')[0])).replace(/^[/\\]+/, '')
        const file = join(pkgDir, rel)
        if (!PDFJS_ASSET_DIRS.includes(rel.split(sep)[0]) || PDFJS_SKIP.test(rel) ||
          !file.startsWith(pkgDir + sep) || !existsSync(file) || !statSync(file).isFile()) {
          return next()
        }
        if (file.endsWith('.wasm')) res.setHeader('Content-Type', 'application/wasm')
        createReadStream(file).pipe(res)
      })
    },
    closeBundle() {
      if (!outDir) return
      for (const dir of PDFJS_ASSET_DIRS) {
        cpSync(join(pkgDir, dir), join(outDir!, PDFJS_PUBLIC_PREFIX, dir), {
          recursive: true,
          filter: (src) => !PDFJS_SKIP.test(src),
        })
      }
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue(), pdfjsAssets()],
  resolve: {
    alias: {
      // Full ESM bundler build (includes the message compiler). The old
      // `dist/vue-i18n.js` path was removed in vue-i18n v11's dist layout.
      'vue-i18n': 'vue-i18n/dist/vue-i18n.esm-bundler.js'
    }
  },
  define: {
    __VUE_I18N_FULL_INSTALL__: true,
    __VUE_I18N_LEGACY_API__: false,
    __INTLIFY_PROD_DEVTOOLS__: false,
  },
  test: {
    environment: 'happy-dom',
    globals: true,
  },
})
