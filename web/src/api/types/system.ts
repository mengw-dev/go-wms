import type { EntityID, PageQuery } from './common'

// ---------- 系统：用户 ----------

export interface UserItem {
  id: EntityID
  username: string
  nickname: string
  status: number
  role_ids: EntityID[]
  created_at: string
  updated_at: string
}

export interface UserListQuery extends PageQuery {
  keyword?: string
  status?: number | ''
}

export interface UserCreateParams {
  username: string
  password: string
  nickname: string
  role_ids: EntityID[]
}

export interface UserUpdateParams {
  nickname?: string
  status?: number
  role_ids?: EntityID[]
}

// ---------- 系统：角色 ----------

export interface RoleItem {
  id: EntityID
  name: string
  perms: string
  remark: string
  created_at: string
  updated_at: string
}

export interface RoleParams {
  name: string
  perms: string
  remark: string
}

export interface RoleListQuery extends PageQuery {
  keyword?: string
}

// ---------- 系统：操作日志 ----------

export interface OperLogItem {
  id: EntityID
  user_id: EntityID
  username: string
  path: string
  method: string
  params: string
  ip: string
  cost_ms: number
  status: number
  result: string
  created_at: string
}

export interface OperLogListQuery extends PageQuery {
  username?: string
}
