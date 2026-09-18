import { post } from './request'

export interface AIChatResp {
  answer: string
}

/** AI 库存问答（后端检索增强：回答基于实时库存数据） */
export function aiChat(question: string) {
  // LLM 生成耗时较长，单独放宽超时（axios 全局默认 20s 不够）
  return post<AIChatResp>('/ai/chat', { question }, { timeout: 45000 })
}
