import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
process.chdir(resolve(__dirname, '..', 'frontend'))
await import('../frontend/node_modules/vite/bin/vite.js')
