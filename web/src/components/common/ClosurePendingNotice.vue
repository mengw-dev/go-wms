<script setup lang="ts">
import { computed } from 'vue'
import { isStale } from '@/utils/pendingOperations'
import { ElMessageBox } from 'element-plus'
import { closureActionText, isClosureCommand } from '@/api/closure'
import type { PendingOperation } from '@/utils/pendingOperations'
const props = defineProps<{ pending: PendingOperation | null; error: string; busy: boolean }>()
const emit = defineEmits<{ recover: []; discard: [] }>()
const command = computed(() => isClosureCommand(props.pending?.businessPayload) ? props.pending.businessPayload : null)
const canRecover = computed(() => !!props.pending && !!command.value && !isStale(props.pending))
async function discard() {
  const owner = JSON.stringify([props.pending?.userId, props.pending?.sessionNamespace, props.pending?.operationKey])
  try { await ElMessageBox.confirm('请先查看原单、库存流水或退货记录确认结果。这里只清除本机待确认记录，不撤销已提交业务。', '核对后放弃', { type: 'warning' }); if (owner === JSON.stringify([props.pending?.userId, props.pending?.sessionNamespace, props.pending?.operationKey])) emit('discard') } catch { /* cancelled */ }
}
</script>
<template>
  <div v-if="pending || error" class="closure-notice">
    <el-alert :title="error || '存在待确认操作，请先用原参数恢复'" type="warning" :closable="false" show-icon />
    <div v-if="pending" class="closure-actions">
      <span>{{ command ? closureActionText[command.action] : '待核对操作' }} · {{ command?.label || '原业务对象' }}<br />{{ command?.data.reason }} · 刷新页面后仍保留原请求</span>
      <el-button type="primary" plain :loading="busy" :disabled="!canRecover" @click="emit('recover')">恢复原操作</el-button>
      <el-button :disabled="busy" @click="discard">已核对，放弃记录</el-button>
    </div>
  </div>
</template>
<style scoped>
.closure-notice { margin-bottom: 12px; }
.closure-actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 8px; }
.closure-actions span { font-size: 12px; color: var(--el-text-color-secondary); }
</style>
