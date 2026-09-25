import { describe, expect, it } from 'vitest'
import type { DemoScenarioResult } from '@/api/types'
import { mergeDemoScenarioResults } from './demoScenario'

function result(name: string, title: string): DemoScenarioResult {
  return {
    name,
    status: 'completed',
    summary: name + ' 完成',
    evidence: [{ label: name + '单据', value: 'NO-1' }],
    links: [{ label: '查看' + name, path: '/demo/' + name }],
    implementation: {
      orchestration: name + '.go',
      business_files: [name + '/service.go'],
      call_chain: [name],
    },
    steps: [{ title, detail: title + '完成', status: 'completed' }],
  }
}

describe('mergeDemoScenarioResults', () => {
  it('合并入库和出库结果且摘要不包含盘点', () => {
    const merged = mergeDemoScenarioResults([result('inbound', '入库'), result('outbound', '出库')])

    expect(merged.name).toBe('full')
    expect(merged.status).toBe('completed')
    expect(merged.summary).toBe('入库、库存和出库业务闭环已完成')
    expect(merged.summary).not.toContain('盘点')
    expect(merged.steps.map((step) => step.title)).toEqual(['入库', '出库'])
    expect(merged.evidence).toHaveLength(2)
    expect(merged.links).toHaveLength(2)
    expect(merged.implementation?.business_files).toEqual(['inbound/service.go', 'outbound/service.go'])
  })

  it('任一流程失败时返回失败状态', () => {
    const outbound = result('outbound', '出库')
    outbound.status = 'failed'
    const merged = mergeDemoScenarioResults([result('inbound', '入库'), outbound])

    expect(merged.status).toBe('failed')
    expect(merged.summary).toBe('入库、库存和出库业务闭环未完全成功')
  })
})
