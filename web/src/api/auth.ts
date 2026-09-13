import { get, post, put } from './request'
import type { ChangePasswordParams, LoginParams, LoginResult, ProfileResult } from './types'

/** 登录（免 token） */
export function login(data: LoginParams) {
  return post<LoginResult>('/login', data)
}

/** 当前用户信息 */
export function getProfile() {
  return get<ProfileResult>('/profile')
}

/** 修改自己的密码 */
export function changePassword(data: ChangePasswordParams) {
  return put<void>('/password', data)
}
