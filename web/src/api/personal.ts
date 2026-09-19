import { get, post } from './request'

/** 持久体验账号（user1..userN）：数据长期保留，登录页"个人空间"由访客自行挑选 */
export interface PersonalAccountInfo {
  username: string
  nickname: string
  /** 仅在领取（claimPersonalAccount）时返回；账号列表接口不下发 */
  password?: string
}

/** 列出全部可选的持久体验账号（免登录接口） */
export function listPersonalAccounts() {
  return get<PersonalAccountInfo[]>('/personal/accounts')
}

/** 领取指定持久账号的登录凭据（免登录接口；用户名非法时后端返回 70008） */
export function claimPersonalAccount(username: string) {
  return post<PersonalAccountInfo>('/personal/login', { username })
}