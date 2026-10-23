<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { InventoryItem } from '@/api/types'
import { useClosureCommands } from '@/composables/useClosureCommands'
import ClosurePendingNotice from '@/components/common/ClosurePendingNotice.vue'
const emit = defineEmits<{ changed: [] }>()
const commands = useClosureCommands(), { pending, busy, error } = commands
const visible = ref(false), row = ref<InventoryItem | null>(null)
const form = reactive({ unfreeze: false, quantity: 1, reason: '' })
const maximum = computed(() => row.value ? (form.unfreeze ? row.value.frozen_quantity ?? 0 : row.value.available_quantity) : 0)
function open(item: InventoryItem) { row.value = item; Object.assign(form, { unfreeze: item.available_quantity <= 0, quantity: 1, reason: '' }); visible.value = true }
async function submit() {
  if (!row.value || form.quantity > maximum.value) return
  const ack = await commands.run({ action: 'freeze', label: `库位 ${row.value.location_code || row.value.location_id} · 批次 ${row.value.batch_no || '—'}`, data: { inventory_id: row.value.id, ...form, reason: form.reason.trim() } })
  if (ack) { visible.value = false; ElMessage.success('库存状态已更新'); emit('changed') }
}
async function recover() { if (await commands.recover()) { visible.value = false; emit('changed') } }
defineExpose({ open })
</script>
<template>
  <ClosurePendingNotice :pending="pending" :busy="busy" :error="error" @recover="recover" @discard="commands.discard" />
  <el-dialog v-model="visible" class="business-dialog" title="库存冻结 / 解冻" width="500px" :close-on-click-modal="!busy">
    <template v-if="row">
      <el-alert title="冻结不减少实物库存。仅可用量可冻结；已分配数量请先通过出库异常释放占用。" type="info" :closable="false" />
      <p>库位 {{ row.location_code || row.location_id }} · 批次 {{ row.batch_no || '—' }}</p>
      <p>可用 {{ row.available_quantity }} · 已分配 {{ row.allocated_quantity }} · 冻结 {{ row.frozen_quantity ?? 0 }}</p>
      <el-form label-position="top" @submit.prevent="submit">
        <el-form-item label="操作"><el-radio-group v-model="form.unfreeze" :disabled="busy || !!pending"><el-radio-button :value="false">冻结</el-radio-button><el-radio-button :value="true">解冻</el-radio-button></el-radio-group></el-form-item>
        <el-form-item :label="`数量（最多 ${maximum}）`"><el-input-number v-model="form.quantity" :min="1" :max="Math.max(1, maximum)" :precision="0" :disabled="busy || !!pending" /></el-form-item>
        <el-form-item label="原因"><el-input v-model="form.reason" type="textarea" maxlength="255" show-word-limit :disabled="busy || !!pending" /></el-form-item>
      </el-form>
      <el-alert v-if="error" :title="error" type="warning" :closable="false" />
    </template>
    <template #footer><el-button :disabled="busy" @click="visible = false">返回</el-button><el-button type="primary" :loading="busy" :disabled="!!pending || maximum < 1 || !form.reason.trim() || form.quantity > maximum" @click="submit">确认{{ form.unfreeze ? '解冻' : '冻结' }}</el-button></template>
  </el-dialog>
</template>
