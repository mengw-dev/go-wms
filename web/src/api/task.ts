import { get } from './request'
import type { PageData, TaskItem, TaskListQuery } from './types'

/** 全部任务列表 */
export function listTasks(params: TaskListQuery) {
  return get<PageData<TaskItem>>('/tasks', params as Record<string, unknown>)
}

export function getTask(id: number | string) {
  return get<TaskItem>(`/tasks/${id}`)
}
