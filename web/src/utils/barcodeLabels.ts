import JsBarcode from 'jsbarcode'
import type { LocationItem, SkuItem } from '@/api/types'

export interface BarcodeLabel {
  key: string
  kind: 'sku' | 'location'
  title: string
  subtitle: string
  value: string
}

export interface EncodedBarcode {
  modules: number
  widthMM: number
  bars: { x: number; width: number }[]
}

export const MAX_LABELS = 200

/** 保留原始载荷（含前导零）。过长时拒绝，不能靠压缩条宽硬塞进标签。 */
export function encodeLabelBarcode(value: string): EncodedBarcode {
  if (typeof value !== 'string' || !/^[\x20-\x7e]{1,64}$/.test(value) || !value.trim()) {
    throw Error('当前 Code 128 标签仅支持1–64位可打印英文字母、数字及符号')
  }
  const output: { encodings?: { data: string }[] } = {}
  JsBarcode(output, value, { format: 'CODE128', displayValue: false, margin: 0 })
  const pattern = output.encodings?.map(encoding => encoding.data).join('') || ''
  if (!pattern || /[^01]/.test(pattern)) throw Error('条码编码失败')

  // 两侧各保留十个模块的静区，每个模块固定0.3mm。
  const modules = pattern.length + 20
  const widthMM = modules * 0.3
  if (widthMM > 80) {
    throw Error('条码过长，无法在当前90×45mm标签保持条宽，请改用更宽的标签样板')
  }
  const bars: EncodedBarcode['bars'] = []
  for (let i = 0; i < pattern.length;) {
    if (pattern[i] !== '1') {
      i++
      continue
    }
    const start = i
    while (pattern[i] === '1') i++
    bars.push({ x: start + 10, width: i - start })
  }
  return { modules, widthMM, bars }
}

export function skuBarcodeLabel(sku: SkuItem): BarcodeLabel {
  return {
    key: `sku:${sku.id}`, kind: 'sku', title: sku.name,
    subtitle: `货品 ${sku.code} · ${sku.unit || '库存单位未设置'}`, value: sku.barcode,
  }
}

export function locationBarcodeLabel(location: LocationItem, warehouse: string): BarcodeLabel {
  return {
    key: `location:${location.id}`, kind: 'location', title: location.code,
    subtitle: `库位 · ${warehouse}`, value: location.code,
  }
}
