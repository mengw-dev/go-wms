import type { EntityID } from './common'

export interface LoginParams {
  username: string
  password: string
  tenant_id?: EntityID
}

export interface LoginResult {
  token: string
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

export interface ProfileResult {
  user_id: EntityID
  username: string
  nickname: string
  roles: string[]
  perms: string[]
}

export interface ChangePasswordParams {
  old_password: string
  new_password: string
}
