import { del, get, post, put } from './request'
import type {
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
  return post<UserItem>('/system/users', data)
}

export function updateUser(id: number, data: UserUpdateParams) {
  return put<void>(`/system/users/${id}`, data)
}

export function deleteUser(id: number) {
  return del<void>(`/system/users/${id}`)
}

export function updateUserStatus(id: number, status: number) {
  return put<void>(`/system/users/${id}/status`, { status })
}

export function resetUserPassword(id: number, password: string) {
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
  return post<RoleItem>('/system/roles', data)
}

export function updateRole(id: number, data: RoleParams) {
  return put<void>(`/system/roles/${id}`, data)
}

export function deleteRole(id: number) {
  return del<void>(`/system/roles/${id}`)
}

// ---------- 操作日志 ----------

export function listOperLogs(params: OperLogListQuery) {
  return get<PageData<OperLogItem>>('/system/oper-logs', params as Record<string, unknown>)
}
