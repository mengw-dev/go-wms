import { copyFile, mkdir, readFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const canonicalPath = resolve(scriptDir, '../public/overview.html')
const targetPath = resolve(scriptDir, '../../docs/index.html')
const checkOnly = process.argv.includes('--check')

if (!existsSync(targetPath)) {
  console.log('overview sync: docs target not present in this build context, skipped')
  process.exit(0)
}

const canonical = await readFile(canonicalPath, 'utf8')
const target = await readFile(targetPath, 'utf8')

if (canonical === target) {
  console.log('overview sync: web/public/overview.html and docs/index.html are identical')
  process.exit(0)
}

if (checkOnly) {
  console.error('overview sync: docs/index.html is stale; run "npm run sync:overview" in web/')
  process.exit(1)
}

await mkdir(dirname(targetPath), { recursive: true })
await copyFile(canonicalPath, targetPath)
console.log('overview sync: copied web/public/overview.html to docs/index.html')
