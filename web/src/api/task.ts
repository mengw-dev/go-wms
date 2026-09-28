import { get, type RequestOptions } from './request'
import type { EntityID,  PageData, TaskItem, TaskListQuery } from './types'

/** 全部任务列表 */
export function listTasks(params: TaskListQuery, options?: RequestOptions) {
  return get<PageData<TaskItem>>('/tasks', params as Record<string, unknown>, options)
}

export function getTask(id: EntityID) {
  return get<TaskItem>(`/tasks/${id}`)
}
