import { describe, expect, it } from 'vitest'
import { encodeLabelBarcode } from './barcodeLabels'

describe('Code128标签编码与尺寸',()=>{
  it('保留前导零，纯数字不转JS数字',()=>{
    const zero=encodeLabelBarcode('000123'),plain=encodeLabelBarcode('123')
    expect(zero.bars).not.toEqual(plain.bars)
    expect(encodeLabelBarcode('000123')).toEqual(zero)
  })
  it('保留两侧静区与固定条宽，不缩小长条码',()=>{
    const result=encodeLabelBarcode('A-01-01')
    expect(result.bars[0]!.x).toBe(10)
    expect(result.bars.at(-1)!.x+result.bars.at(-1)!.width).toBeLessThanOrEqual(result.modules-10)
    expect(result.widthMM).toBe(result.modules*0.3)
    expect(()=>encodeLabelBarcode('A'.repeat(64))).toThrow('条码过长')
  })
  it.each(['','   ','中文','A\nB','A\tB','A'.repeat(65)])('不支持的内容明确拒绝：%j',value=>{
    expect(()=>encodeLabelBarcode(value)).toThrow()
  })
})
