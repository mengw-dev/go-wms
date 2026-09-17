import { onBeforeUnmount, onMounted } from 'vue'
import { onDataChanged } from '@/utils/events'

// 页面自动刷新：定时轮询 + 演示流程完成后立即刷新。
export function useAutoRefresh(refresh: () => void | Promise<void>, intervalMs = 5000): void {
  let timer: number | undefined
  let stop: (() => void) | undefined

  onMounted(() => {
    timer = window.setInterval(() => void refresh(), intervalMs)
    stop = onDataChanged(() => void refresh())
  })

  onBeforeUnmount(() => {
    if (timer !== undefined) window.clearInterval(timer)
    stop?.()
  })
}