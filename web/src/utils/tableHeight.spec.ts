import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clampTableHeight, maxTableHeight, readTableHeight, saveTableHeight, tableHeightKey } from './tableHeight'

let values: Map<string, string>
beforeEach(() => {
  values = new Map()
  vi.stubGlobal('localStorage', { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value), removeItem: (key: string) => values.delete(key) })
})
afterEach(() => vi.unstubAllGlobals())
describe('业务表格高度偏好', () => {
  it('按页面、表格及手机/桌面分别保存，恢复默认只清除当前项', () => {
    const key = tableHeightKey('inventory-detail', false), phone = tableHeightKey('inventory-detail', true), summary = tableHeightKey('inventory-summary', false)
    saveTableHeight(key, 700); saveTableHeight(phone, 500); saveTableHeight(summary, 400)
    expect(readTableHeight(key, 800)).toBe(700); expect(readTableHeight(phone, 800)).toBe(500)
    saveTableHeight(key, null)
    expect(readTableHeight(key, 800)).toBeNull(); expect(readTableHeight(phone, 800)).toBe(500); expect(readTableHeight(summary, 800)).toBe(400)
  })
  it.each(['', ' ', '-3', '0', 'NaN', 'Infinity', 'abc', '{}'])('无效偏好 %j 回到自动高度', value => {
    values.set('key', value); expect(readTableHeight('key', 800)).toBeNull()
  })
  it('约束最小高度和两屏上限，缩屏后钳制，不允许无限拉长', () => {
    expect(clampTableHeight(-100, 800)).toBe(160)
    expect(clampTableHeight(99999, 800)).toBe(1600)
    expect(maxTableHeight(2000)).toBe(2400)
    values.set('key', '1500'); expect(readTableHeight('key', 600)).toBe(1200)
  })
  it('存储被禁用时读取回默认，调整和恢复不抛错', () => {
    vi.stubGlobal('localStorage', { getItem: () => { throw Error('blocked') }, setItem: () => { throw Error('blocked') }, removeItem: () => { throw Error('blocked') } })
    expect(readTableHeight('key', 800)).toBeNull()
    expect(() => saveTableHeight('key', 400)).not.toThrow()
    expect(() => saveTableHeight('key', null)).not.toThrow()
  })
})
