<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import type { Wave } from '@/api/wave'
import { useClosureCommands } from '@/composables/useClosureCommands'
import ClosurePendingNotice from '@/components/common/ClosurePendingNotice.vue'
const props = defineProps<{ wave: Wave; blocked: boolean }>()
const emit = defineEmits<{ changed: []; pendingChange: [value: boolean] }>()
const commands = useClosureCommands(), { pending, busy, error } = commands
const dialog = ref(false), form = reactive({ confirm_returned: false, reason: '' })
watch(() => !!pending.value || busy.value, value => emit('pendingChange', value), { immediate: true })
watch(() => props.wave.id, () => { dialog.value = false })
function open() { Object.assign(form, { confirm_returned: false, reason: '' }); dialog.value = true }
async function submit() {
  if (!form.confirm_returned || !form.reason.trim() || props.blocked) return
  if (await commands.run({ action: 'abort', label: props.wave.wave_no, wave_id: props.wave.id, data: { ...form, reason: form.reason.trim() } })) { dialog.value = false; emit('changed') }
}
async function recover() { if (await commands.recover()) { dialog.value = false; emit('changed') } }
</script>
<template>
  <div class="no-print wave-abort-panel">
    <ClosurePendingNotice v-if="!dialog" :pending="pending" :busy="busy" :error="error" @recover="recover" @discard="commands.discard" />
    <el-button v-if="!['COMPLETED', 'CANCELLED', 'ABORTED'].includes(wave.status)" v-permission="'wms:outbound:exception'" type="warning" plain :disabled="blocked || busy || !!pending" @click="open">异常结束波次</el-button>
    <el-alert v-if="wave.status === 'ABORTED'" type="warning" :closable="false" :title="`波次已异常结束：${wave.abort_reason || '请核对现场记录'}。已发成员保留；未发成员已恢复普通任务。`" />
    <el-dialog v-model="dialog" title="异常结束波次" class="business-dialog" width="560px" :close-on-click-modal="!busy">
      <ClosurePendingNotice :pending="pending" :busy="busy" :error="error" @recover="recover" @discard="commands.discard" />
      <el-alert title="已发货订单和扣库结果保持。未发订单解除波次绑定，原库存占用保持，之后可从出库单登记短缺/货损或继续普通拣货。结束前必须把未发实物退回原库位。" type="warning" :closable="false" />
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item label="结束原因"><el-input v-model="form.reason" type="textarea" maxlength="255" show-word-limit :disabled="busy || !!pending" /></el-form-item>
        <el-form-item><el-checkbox v-model="form.confirm_returned" :disabled="busy || !!pending">已确认所有未发实物退回原库位，并通知现场作业员停止该波次</el-checkbox></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="busy" @click="dialog = false">返回</el-button><el-button type="warning" :disabled="!form.confirm_returned || !form.reason.trim() || !!pending || blocked" :loading="busy" @click="submit">确认异常结束</el-button></template>
    </el-dialog>
  </div>
</template>
<style scoped>
.wave-abort-panel { margin-bottom: 12px; }
.wave-abort-panel :deep(.el-alert), .wave-abort-panel :deep(.el-form) { margin-top: 12px; }
.wave-abort-panel :deep(.el-checkbox), .wave-abort-panel :deep(.el-checkbox__label) { white-space: normal; height: auto; line-height: 1.6; }
</style>
