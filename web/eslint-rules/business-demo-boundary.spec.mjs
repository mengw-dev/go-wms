import { fileURLToPath, URL } from 'node:url'
import path from 'node:path'
import { ESLint } from 'eslint'
import { beforeAll, describe, expect, it } from 'vitest'

const root = fileURLToPath(new URL('../', import.meta.url))
const lint = new ESLint({ cwd: root, overrideConfigFile: path.join(root, 'eslint.config.js') })
async function violations(code, file = 'src/views/inbound/GuardFixture.vue') {
  const source = file.endsWith('.vue') ? `<script setup lang="ts">\n${code}\n</script><template><div /></template>` : code
  const results = await lint.lintText(source, { filePath: path.join(root, file) })
  expect(results[0]?.messages.filter((message) => message.fatal)).toEqual([])
  return results[0].messages.filter((message) => message.ruleId === 'architecture/business-demo-boundary')
}

// Load the actual flat config and Vue/TS parser once as suite setup, outside case timing.
beforeAll(async () => { expect(await violations("import type { InboundOrderItem } from '@/api/types'")).toHaveLength(0) })

describe('实际 ESLint 配置的依赖护栏', () => {
  it.each([
    "import { useGuideStore } from '@/stores/guide'",
    "import type { GuideStep } from '../../guide/types'",
    "import { run } from '@/views/../composables/demo/useDemoRunner'",
    "import '/src/components/demo/ManualGuide.vue'",
    "export * from '../../guide/events'",
    "export { useGuideStore } from '@/stores/guide'",
    "const loader = () => import('@/composables/demo/useGuideRunner')",
    "const loader = () => import(`@/guide/definitions`)",
    "type Step = import('@/guide/types').GuideStep",
    "import guide = require('@/stores/guide')",
    "import * as controls from '@/utils/events'",
    "import { runDemoScenarioInConsole as run } from '@/utils/events'",
    "const loader = (modulePath: string) => import(modulePath)",
  ])('拒绝业务模块的反向依赖：%s', async (code) => {
    expect(await violations(code)).toHaveLength(1)
  })

  it.each([
    "import { BUSINESS_EVENTS, emitBusinessEvent } from '@/events/businessEvents'",
    "import { getInboundOrder } from '@/api/inbound'",
    "import type { InboundOrderItem } from '@/api/types'",
    "import { onDataChanged as subscribe } from '@/utils/events'",
    "export { DATA_CHANGED_EVENT } from '@/utils/events'",
  ])('保留业务 API 和事件契约：%s', async (code) => {
    expect(await violations(code)).toHaveLength(0)
  })

  it.each(['src/layouts/Layout.vue', 'src/views/login/index.vue', 'src/components/DemoConsole.vue', 'src/views/demo/index.vue', 'src/composables/demo/fixture.ts'])('保留装配和演示入口：%s', async (file) => {
    expect(await violations("import { useGuideStore } from '@/stores/guide'", file)).toHaveLength(0)
  })

  it('共享类型出口允许再导出 Demo 契约', async () => {
    expect(await violations("export * from './demo'", 'src/api/types/index.ts')).toHaveLength(0)
  })
  it('Guide/Demo 通过业务 API 与业务事件交互，不能直接依赖业务页面', async () => {
    const file = 'src/guide/guardFixture.ts'
    expect(await violations("import { getInboundOrder } from '@/api/inbound'", file)).toHaveLength(0)
    expect(await violations("import page from '@/views/inbound/InboundOrders.vue'", file)).toHaveLength(1)
  })
  it('业务事件层不能反向依赖 store', async () => {
    expect(await violations("import { useAuthStore } from '@/stores/auth'", 'src/events/businessEvents.ts')).toHaveLength(1)
  })
  it('测试允许组装跨模块 fixture，生产规则仍默认启用', async () => {
    expect(await violations("import { useGuideStore } from '@/stores/guide'", 'src/events/fixture.spec.ts')).toHaveLength(0)
    expect(await violations("import { useGuideStore } from '@/stores/guide'", 'src/events/fixture.ts')).toHaveLength(1)
  })
})
