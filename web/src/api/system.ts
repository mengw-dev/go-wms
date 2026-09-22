import { del, get, post, put } from './request'
import type {
  EntityID,
  OperLogItem,
  OperLogListQuery,
  PageData,
  RoleItem,
  RoleListQuery,
  RoleParams,
  UserCreateParams,
  UserItem,
  UserListQuery,
  UserUpdateParams,
} from './types'

// ---------- 用户 ----------

export function listUsers(params: UserListQuery) {
  return get<PageData<UserItem>>('/system/users', params as Record<string, unknown>)
}

export function createUser(data: UserCreateParams) {
  return post<void>('/system/users', data)
}

export function updateUser(id: EntityID, data: UserUpdateParams) {
  return put<void>(`/system/users/${id}`, data)
}

export function deleteUser(id: EntityID) {
  return del<void>(`/system/users/${id}`)
}

export function updateUserStatus(id: EntityID, status: number) {
  return put<void>(`/system/users/${id}/status`, { status })
}

export function resetUserPassword(id: EntityID, password: string) {
  return put<void>(`/system/users/${id}/password`, { password })
}

// ---------- 角色 ----------

/** 全量角色（不分页），用于下拉/多选 */
export function listAllRoles() {
  return get<RoleItem[]>('/system/roles/all')
}

export function listRoles(params: RoleListQuery) {
  return get<PageData<RoleItem>>('/system/roles', params as Record<string, unknown>)
}

export function createRole(data: RoleParams) {
  return post<void>('/system/roles', data)
}

export function updateRole(id: EntityID, data: RoleParams) {
  return put<void>(`/system/roles/${id}`, data)
}

export function deleteRole(id: EntityID) {
  return del<void>(`/system/roles/${id}`)
}

// ---------- 操作日志 ----------

export function listOperLogs(params: OperLogListQuery) {
  return get<PageData<OperLogItem>>('/system/oper-logs', params as Record<string, unknown>)
}
