<script setup lang="ts">
import { onScopeDispose, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { getExcel } from '@/api/request'
import { useAuthStore } from '@/stores/auth'
import { exportQuery } from '@/utils/exportQuery'

const props = defineProps<{ endpoint: string; params: object; title: string; label?: string }>()
const auth = useAuthStore()
const loading = ref(false)
let alive = true
let sequence = 0
let controller: typeof window.AbortController.prototype | null = null
const urls = new Set<string>()
const identity = () => JSON.stringify([auth.sessionVersion,auth.token,auth.demoSessionId])
function cancel() {
  sequence++
  controller?.abort()
  controller = null
  loading.value = false
}
watch(identity, cancel)
onScopeDispose(() => {
  alive = false
  cancel()
  for (const url of urls) window.URL.revokeObjectURL(url)
  urls.clear()
})

async function download() {
  if (loading.value || !auth.token) return
  const owner = identity()
  const current = ++sequence
  controller = new window.AbortController()
  const signal = controller.signal
  const params = exportQuery(props.params)
  loading.value = true
  try {
    const blob = await getExcel(props.endpoint,params,signal)
    if (!alive || signal.aborted || current !== sequence || owner !== identity()) return
    const url = window.URL.createObjectURL(blob)
    urls.add(url)
    const link = document.createElement('a')
    link.href = url
    link.download = `${props.title}-${new Date().toISOString().slice(0,10)}.xlsx`
    document.body.append(link)
    link.click()
    link.remove()
    // Give the browser time to start the download before releasing the Blob window.URL.
    window.setTimeout(() => {
      if (urls.delete(url)) window.URL.revokeObjectURL(url)
    },1500)
    ElMessage.success('文件已生成，请查看浏览器下载记录')
  } catch (error) {
    if (alive && !signal.aborted && current === sequence && owner === identity()) {
      ElMessage.error(error instanceof Error ? error.message : '导出失败，请稍后重试')
    }
  } finally {
    if (alive && current === sequence) { loading.value = false; controller = null }
  }
}
</script>

<template>
  <el-tooltip content="按点击时的筛选条件导出全部匹配记录，最多10000行；超限请缩小范围" placement="top">
    <el-button :loading="loading" @click="download">{{ label || '导出Excel' }}</el-button>
  </el-tooltip>
</template>
