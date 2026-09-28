/**
 * 入库 Excel 导入：文件选择、上传、进度轮询与清理。
 *
 * 轮询采用"请求 → 请求结束 → 等待 → 下一次请求"的串行方式，
 * 不使用 setInterval，保证同一任务同一时间最多一个状态请求；
 * token 用于让旧任务的迟到响应失效。
 */
import { onUnmounted, reactive, ref } from 'vue'
import { ElMessage, genFileId } from 'element-plus'
import type { UploadFile, UploadRawFile } from 'element-plus'
import { getImportStatus, importInboundExcel } from '@/api/inbound'
import type { ImportTaskItem } from '@/api/types'

const POLL_INTERVAL_MS = 2000

export function useInboundImport(onCompleted: () => void | Promise<void>) {
  const importDialog = reactive({ visible: false, uploading: false })
  const importFile = ref<File | null>(null)
  const importInfo = ref<ImportTaskItem | null>(null)

  let pollToken = 0
  let pollTimer: number | undefined

  function onFileChange(file: UploadFile) {
    importFile.value = (file.raw as File) ?? null
  }

  function onFileRemove() {
    importFile.value = null
  }

  function handleExceed(files: File[]) {
    const raw = files[0] as UploadRawFile
    raw.uid = genFileId()
    importFile.value = raw as unknown as File
  }

  function stopPolling() {
    pollToken += 1
    if (pollTimer !== undefined) {
      window.clearTimeout(pollTimer)
      pollTimer = undefined
    }
  }

  function startPolling(taskId: string) {
    stopPolling()
    const token = pollToken

    const tick = async () => {
      if (token !== pollToken) return
      let info: ImportTaskItem
      try {
        info = await getImportStatus(taskId)
      } catch {
        // 轮询出错时静默停止，避免持续弹出错误提示
        return
      }
      if (token !== pollToken) return

      importInfo.value = info
      if (info.status === 'COMPLETED' || info.status === 'FAILED') {
        if (info.status === 'COMPLETED') {
          ElMessage.success(`导入完成：成功 ${info.success_rows} 条，失败 ${info.fail_rows} 条`)
        }
        void onCompleted()
        return
      }
      pollTimer = window.setTimeout(() => { void tick() }, POLL_INTERVAL_MS)
    }

    void tick()
  }

  function openImport() {
    importFile.value = null
    importInfo.value = null
    importDialog.visible = true
  }

  function closeImport() {
    stopPolling()
    importDialog.visible = false
  }

  async function startImport() {
    if (!importFile.value) {
      ElMessage.warning('请先选择 Excel 文件')
      return
    }
    importDialog.uploading = true
    importInfo.value = null
    try {
      const resp = await importInboundExcel(importFile.value)
      ElMessage.success('文件已上传，开始解析导入')
      importInfo.value = {
        task_id: resp.task_id,
        status: 'PENDING',
        file_name: importFile.value.name,
        total_rows: 0,
        success_rows: 0,
        fail_rows: 0,
        error_msg: '',
      }
      startPolling(resp.task_id)
    } finally {
      importDialog.uploading = false
    }
  }

  onUnmounted(stopPolling)

  return {
    importDialog,
    importFile,
    importInfo,
    openImport,
    closeImport,
    startImport,
    onFileChange,
    onFileRemove,
    handleExceed,
  }
}
