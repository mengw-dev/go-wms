<script setup lang="ts">
import type { InputInstance } from 'element-plus'
import { ref, watch } from 'vue'
const props = withDefaults(defineProps<{ label: string; placeholder: string; disabled?: boolean; loading?: boolean }>(), { disabled: false, loading: false })
const model = defineModel<string>({ required: true })
const emit = defineEmits<{ scan: [] }>()
const input = ref<InputInstance>()
function enter(event: KeyboardEvent) {
  // IME Enter must finish composing text; it is not a scanner terminator.
  if (event.key !== 'Enter' || event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  event.stopPropagation()
  if (props.disabled || props.loading || event.repeat) return
  emit('scan')
}
// Keep keyboard-scanner focus through the read request. Select the previous
// value for the next scan, without stealing focus from another field.
watch(() => props.loading, loading => {
  if (!loading && input.value?.input === document.activeElement) input.value?.select()
}, { flush: 'post' })
defineExpose({ focus: () => input.value?.focus() })
</script>

<template>
  <div class="scan-field">
    <el-input ref="input" v-model="model" :aria-label="label" :placeholder="placeholder" :disabled="disabled" :readonly="loading" :maxlength="64" clearable autocomplete="off" spellcheck="false" @keydown="enter" />
    <el-button :disabled="disabled || !model.trim()" :loading="loading" @click="emit('scan')">核对</el-button>
  </div>
</template>

<style scoped>
.scan-field { display: flex; width: 100%; gap: 8px; align-items: center; }
.scan-field .el-input { flex: 1; min-width: 0; }
</style>
