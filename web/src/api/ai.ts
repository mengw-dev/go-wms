import { post } from './request'

export interface AIChatResp {
  answer: string
}

/** AI 库存问答（后端检索增强：回答基于实时库存数据） */
export function aiChat(question: string, signal?: AbortSignal) {
  // Backend total <=25s, proxy 30s; leave transport headroom at the browser.
  return post<AIChatResp>('/ai/chat', { question }, { timeout: 35000, signal, silentError: true })
}
