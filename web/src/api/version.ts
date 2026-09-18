import { get } from './request'
import type { VersionResult } from './types'

/** 服务版本与特性开关（公开接口，无需登录） */
export function getVersion() {
  return get<VersionResult>('/version')
}
