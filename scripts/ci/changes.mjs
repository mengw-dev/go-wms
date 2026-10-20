import { execFileSync } from 'node:child_process'
import { appendFileSync } from 'node:fs'
import { classifyPaths } from './policy.mjs'

const base = process.env.BASE_SHA || ''
const head = process.env.HEAD_SHA || ''
let scope
let reason
try {
  if (![base, head].every(value => /^[a-f0-9]{40}$/i.test(value) && !/^0+$/.test(value))) {
    throw new Error('没有可比较的提交')
  }
  for (const sha of [base, head]) execFileSync('git', ['cat-file', '-e', `${sha}^{commit}`], { stdio: 'pipe' })
  const diff = execFileSync('git', ['diff', '--name-only', '--no-renames', '-z', base, head, '--'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
  const paths = diff.split('\0').filter(Boolean)
  scope = classifyPaths(paths)
  reason = `检查 ${paths.length} 个变更路径（包含删除与重命名前路径）`
} catch {
  scope = { backend: true, frontend: true, integration: true }
  reason = '比较基线不可用，保守执行全部检查'
}

const output = Object.entries(scope).map(([key, value]) => `${key}=${value}\n`).join('')
if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, output)
if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY,
  `### CI 改动范围\n\n${reason}\n\n| 后端检查 | 前端检查 | 容器与真实 E2E |\n| --- | --- | --- |\n| ${scope.backend} | ${scope.frontend} | ${scope.integration} |\n`)
console.log(reason)
console.log(output.trim())
