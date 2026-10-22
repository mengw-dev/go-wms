<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { clampTableHeight, maxTableHeight, MIN_TABLE_HEIGHT, readTableHeight, saveTableHeight, tableHeightKey } from '@/utils/tableHeight'

const props = withDefaults(defineProps<{ storageKey: string; active?: boolean }>(), { active: true })
const area = ref<HTMLElement>(), id = useId()
const compact = ref(window.innerWidth <= 768), viewport = ref(window.innerHeight)
const key = computed(() => tableHeightKey(props.storageKey, compact.value))
const height = ref<number | null>(readTableHeight(key.value, viewport.value))
const measured = ref(0), dragging = ref(false)
const style = computed(() => height.value !== null && props.active ? { '--user-table-height': `${height.value}px` } : undefined)
let observer: ResizeObserver | undefined, frame = 0
let drag: { pointer: number; y: number; height: number; scroll: number; main: HTMLElement; previous: number | null; clientY: number } | undefined

function change(value: number | null, persist = true) {
  height.value = value === null ? null : clampTableHeight(value, viewport.value)
  if (persist) saveTableHeight(key.value, height.value)
}
function stop(cancel = false) {
  if (!drag) return
  if (cancel) height.value = drag.previous
  else saveTableHeight(key.value, height.value)
  drag = undefined; dragging.value = false
  window.cancelAnimationFrame(frame)
  document.body.classList.remove('table-height-dragging')
}
function updateDrag() {
  if (!drag) return
  change(drag.height + drag.clientY - drag.y + drag.main.scrollTop - drag.scroll, false)
}
function autoScroll() {
  if (!drag) return
  const bounds = drag.main.getBoundingClientRect()
  const delta = drag.clientY > bounds.bottom - 38 ? 8 : drag.clientY < bounds.top + 38 ? -8 : 0
  if (delta && (height.value || 0) < maxTableHeight(viewport.value)) {
    drag.main.scrollTop += delta
    updateDrag()
  }
  frame = window.requestAnimationFrame(autoScroll)
}
function start(event: PointerEvent) {
  if (!props.active || event.button !== 0 || !event.isPrimary || drag) return
  const main = area.value?.closest<HTMLElement>('.main')
  if (!main || !area.value) return
  const target = event.currentTarget as HTMLElement
  target.focus({ preventScroll: true })
  target.setPointerCapture(event.pointerId)
  drag = { pointer: event.pointerId, y: event.clientY, clientY: event.clientY, height: area.value.getBoundingClientRect().height, scroll: main.scrollTop, main, previous: height.value }
  dragging.value = true
  document.body.classList.add('table-height-dragging')
  frame = window.requestAnimationFrame(autoScroll)
  event.preventDefault()
}
function move(event: PointerEvent) {
  if (!drag || event.pointerId !== drag.pointer) return
  drag.clientY = event.clientY; updateDrag()
}
function end(event: PointerEvent, cancel = false) {
  if (drag?.pointer === event.pointerId) stop(cancel)
}
function keyboard(event: KeyboardEvent) {
  if (event.key === 'Escape') { stop(true); return }
  if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  if (event.key === 'Home') change(null)
  else if (event.key === 'End') change(viewport.value - 100)
  else change((height.value ?? measured.value) + (event.key === 'ArrowDown' ? 1 : -1) * (event.shiftKey ? 120 : 40))
}
function resize() {
  stop(true)
  viewport.value = window.innerHeight
  compact.value = window.innerWidth <= 768
  // Fit temporarily smaller windows without overwriting the user's saved size.
  height.value = readTableHeight(key.value, viewport.value)
}
watch(() => props.active, () => stop(true))
onMounted(() => {
  observer = new ResizeObserver(entries => { measured.value = Math.round(entries[0]?.borderBoxSize[0]?.blockSize ?? area.value?.getBoundingClientRect().height ?? 0) })
  if (area.value) observer.observe(area.value)
  window.addEventListener('resize', resize)
})
onBeforeUnmount(() => { stop(true); observer?.disconnect(); window.removeEventListener('resize', resize) })
</script>

<template>
  <div :id="id" ref="area" class="business-table-area" :class="{ 'is-resized': height !== null && active, 'is-dragging': dragging }" :style="style" :data-height-key="storageKey">
    <slot />
    <div class="table-height-tools">
      <div class="table-height-handle" role="separator" tabindex="0" aria-label="拖动调整表格高度" aria-orientation="horizontal" :aria-controls="id" :aria-valuemin="height === null ? 0 : MIN_TABLE_HEIGHT" :aria-valuemax="maxTableHeight(viewport)" :aria-valuenow="measured" :aria-valuetext="height === null ? '自动适配一屏' : `${measured} 像素`" @pointerdown="start" @pointermove="move" @pointerup="end($event)" @pointercancel="end($event, true)" @lostpointercapture="end($event)" @keydown="keyboard" @dblclick="change(null)"><i></i><span>拖动调整高度</span></div>
      <button type="button" title="增加160像素" @click="change((height ?? measured) + 160)">加高</button>
      <button type="button" title="将结果区拉高到接近一屏，页面可上下滚动" @click="change(viewport - 100)">一屏高度</button>
      <button type="button" :disabled="height === null" @click="change(null)">恢复默认</button>
    </div>
  </div>
</template>
