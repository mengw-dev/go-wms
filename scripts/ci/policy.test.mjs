import { test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, renameSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { classifyPaths, shouldDeploy, validateResults } from './policy.mjs'

test('business source scopes, docs, shared inputs and unknown files', () => {
  for (const paths of [['web/src/views/inventory/Inventory.vue'], ['web/package-lock.json'], ['web/tests/e2e/ui-layout.spec.ts'], ['web/public/overview.html']]) {
    assert.deepEqual(classifyPaths(paths), { backend: false, frontend: true, integration: true })
  }
  for (const path of ['internal/modules/inbound/service/order.go', 'go.mod', 'go.sum', 'migrations/versions/000001.up.sql']) {
    assert.deepEqual(classifyPaths([path]), { backend: true, frontend: false, integration: true })
  }
  assert.deepEqual(classifyPaths(['docs/development.md', 'docs/index.html', 'README.md', 'LICENSE']), { backend: false, frontend: false, integration: false })
  for (const path of ['.github/workflows/ci.yml', '.github/workflows/deploy.yml', 'scripts/ci/policy.mjs', 'Dockerfile', 'deploy/Caddyfile', '.env.example', 'new-build-input']) {
    assert.deepEqual(classifyPaths([path]), { backend: true, frontend: true, integration: true })
  }
  assert.deepEqual(classifyPaths(['web/src/a.vue', 'internal/a.go']), { backend: true, frontend: true, integration: true })
})

test('result gate permits only planned skips and fails on missing, failed or cancelled checks', () => {
  for (const [backend, frontend] of [[false, false], [false, true], [true, false], [true, true]]) {
    const integration = backend || frontend
    const needs = { changes: { result: 'success', outputs: { backend: String(backend), frontend: String(frontend), integration: String(integration) } },
      backend: { result: backend ? 'success' : 'skipped' }, frontend: { result: frontend ? 'success' : 'skipped' },
      containers: { result: integration ? 'success' : 'skipped' }, e2e: { result: integration ? 'success' : 'skipped' } }
    assert.doesNotThrow(() => validateResults(needs))
    for (const job of ['backend', 'frontend', 'containers', 'e2e']) {
      for (const result of ['failure', 'cancelled', 'missing']) assert.throws(() => validateResults({ ...needs, [job]: { result } }))
      assert.throws(() => validateResults({ ...needs, [job]: { result: needs[job].result === 'success' ? 'skipped' : 'success' } }))
    }
    assert.throws(() => validateResults({ ...needs, changes: { result: 'failure' } }))
    assert.throws(() => validateResults({ ...needs, changes: { result: 'success', outputs: {} } }))
  }
})

test('deployment requires the final gate and both integration checks; docs and duplicates cannot deploy', () => {
  const jobs = ['CI result', 'Containers (build)', 'E2E (Playwright)'].map(name => ({ name, conclusion: 'success' }))
  assert.equal(shouldDeploy(jobs), true)
  for (const conclusion of ['failure', 'skipped', 'cancelled', null]) {
    for (let i = 0; i < jobs.length; i++) assert.equal(shouldDeploy(jobs.map((job, index) => index === i ? { ...job, conclusion } : job)), false)
  }
  assert.equal(shouldDeploy([]), false)
  assert.equal(shouldDeploy(jobs.slice(1)), false)
  assert.equal(shouldDeploy([...jobs, jobs[0]]), false)
})

test('real Git CLI compares the whole push and includes deleted/renamed source paths', () => {
  const directory = mkdtempSync(join(tmpdir(), 'wms-ci-policy-'))
  const script = fileURLToPath(new URL('./changes.mjs', import.meta.url))
  const git = (...args) => execFileSync('git', args, { cwd: directory, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  try {
    git('init', '-q')
    git('config', 'user.name', 'CI policy test')
    git('config', 'user.email', 'ci-test@example.invalid')
    mkdirSync(join(directory, 'web'), { recursive: true })
    mkdirSync(join(directory, 'docs'), { recursive: true })
    writeFileSync(join(directory, 'web', 'page.vue'), 'initial')
    git('add', '.'); git('commit', '-qm', 'initial')
    const base = git('rev-parse', 'HEAD')
    writeFileSync(join(directory, 'web', 'page.vue'), 'changed')
    git('commit', '-qam', 'frontend')
    const run = (baseSHA, headSHA) => {
      const output = join(directory, 'outputs')
      writeFileSync(output, '')
      execFileSync(process.execPath, [script], { cwd: directory, env: { ...process.env, BASE_SHA: baseSHA, HEAD_SHA: headSHA, GITHUB_OUTPUT: output, GITHUB_STEP_SUMMARY: '' }, stdio: 'pipe' })
      return readFileSync(output, 'utf8')
    }
    assert.equal(run(base, git('rev-parse', 'HEAD')), 'backend=false\nfrontend=true\nintegration=true\n')
    // Last commit is docs-only, but the entire push still contains a frontend edit.
    writeFileSync(join(directory, 'docs', 'note.md'), 'docs')
    git('add', 'docs'); git('commit', '-qm', 'docs')
    const docsHead = git('rev-parse', 'HEAD')
    assert.equal(run(base, docsHead), 'backend=false\nfrontend=true\nintegration=true\n')
    assert.equal(run(git('rev-parse', 'HEAD^'), docsHead), 'backend=false\nfrontend=false\nintegration=false\n')
    renameSync(join(directory, 'web', 'page.vue'), join(directory, 'docs', 'page.md'))
    git('add', 'web', 'docs'); git('commit', '-qm', 'move source to docs')
    assert.equal(run(docsHead, git('rev-parse', 'HEAD')), 'backend=false\nfrontend=true\nintegration=true\n')
    assert.equal(run('0'.repeat(40), docsHead), 'backend=true\nfrontend=true\nintegration=true\n')
    assert.equal(run('f'.repeat(40), docsHead), 'backend=true\nfrontend=true\nintegration=true\n')
  } finally {
    assert.equal(dirname(resolve(directory)), resolve(tmpdir()))
    assert.ok(directory.startsWith(join(tmpdir(), 'wms-ci-policy-')))
    rmSync(directory, { recursive: true, force: true })
  }
})
