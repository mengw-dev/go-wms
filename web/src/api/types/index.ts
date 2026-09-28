/**
 * API 类型按领域拆分，统一从这里再导出。
 * 调用方继续使用 `import type { X } from '@/api/types'` 即可，无需关心具体文件。
 */
export * from './common'
export * from './auth'
export * from './system'
export * from './basic'
export * from './inventory'
export * from './task'
export * from './inbound'
export * from './outbound'
export * from './stocktake'
export * from './demo'
