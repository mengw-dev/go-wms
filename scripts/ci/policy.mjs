// Keep unknown paths conservative: a new build input must never silently skip checks.
export function classifyPaths(paths) {
  let backend = false
  let frontend = false
  for (const path of paths) {
    if (path.endsWith('.go') || /^(go\.(mod|sum)|cmd\/|internal\/|migrations\/)/.test(path)) backend = true
    else if (path.startsWith('web/')) frontend = true
    else if (path.startsWith('docs/') || /^[^/]+\.md$/i.test(path) || /^LICENSE(?:\.[^/]+)?$/.test(path)) continue
    else { backend = true; frontend = true }
  }
  return { backend, frontend, integration: backend || frontend }
}

export function validateResults(needs) {
  if (needs.changes?.result !== 'success') throw new Error('改动范围检查未成功')
  const scope = needs.changes.outputs
  for (const key of ['backend', 'frontend', 'integration']) {
    if (!['true', 'false'].includes(scope[key])) throw new Error(`无效的检查范围：${key}`)
  }
  if ((scope.integration === 'true') !== (scope.backend === 'true' || scope.frontend === 'true')) {
    throw new Error('业务集成检查范围与源码范围不一致')
  }
  for (const [job, required] of Object.entries({ backend: scope.backend, frontend: scope.frontend, containers: scope.integration, e2e: scope.integration })) {
    const expected = required === 'true' ? 'success' : 'skipped'
    if (needs[job]?.result !== expected) throw new Error(`${job} 应为 ${expected}，实际为 ${needs[job]?.result || 'missing'}`)
  }
}

export function shouldDeploy(jobs) {
  // workflow_run can also report a successful docs-only run. It must not deploy.
  const checks = ['CI result', 'Containers (build)', 'E2E (Playwright)']
  return checks.every(name => {
    const matches = jobs.filter(job => job.name === name)
    return matches.length === 1 && matches[0].conclusion === 'success'
  })
}
