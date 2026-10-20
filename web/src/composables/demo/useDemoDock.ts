import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'

const STORAGE_KEY = 'WMS_DEMO_DOCK_POSITION_V1'

/** Only the handle moves the dock; action buttons retain their normal click behavior. */
export function useDemoDock(element: Ref<HTMLElement | undefined>, floating: Ref<boolean>) {
  const position = ref<{ x: number; y: number } | null>(null)
  const dragging = ref(false)
  let pointer: { id: number; x: number; y: number; left: number; top: number } | null = null
  let observer: ResizeObserver | undefined

  const style = computed(() => floating.value && position.value
    ? { left: `${position.value.x}px`, top: `${position.value.y}px`, right: 'auto', bottom: 'auto' }
    : undefined)

  function clamp(x: number, y: number) {
    const box = element.value?.getBoundingClientRect()
    return { x: Math.max(8, Math.min(x, window.innerWidth - (box?.width ?? 0) - 8)),
      y: Math.max(8, Math.min(y, window.innerHeight - (box?.height ?? 0) - 8)) }
  }
  function persist() {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(position.value)) } catch { /* Position is optional. */ }
  }
  function fit() {
    if (floating.value && position.value) position.value = clamp(position.value.x, position.value.y)
  }
  function start(event: PointerEvent) {
    if (!floating.value || event.button !== 0 || !event.isPrimary) return
    const box = element.value?.getBoundingClientRect()
    if (!box) return
    pointer = { id: event.pointerId, x: event.clientX, y: event.clientY, left: box.left, top: box.top }
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
    dragging.value = true
    event.preventDefault()
  }
  function move(event: PointerEvent) {
    if (!pointer || event.pointerId !== pointer.id) return
    position.value = clamp(pointer.left + event.clientX - pointer.x, pointer.top + event.clientY - pointer.y)
  }
  function finish(event: PointerEvent) {
    if (!pointer || event.pointerId !== pointer.id) return
    pointer = null
    dragging.value = false
    persist()
  }
  function keydown(event: KeyboardEvent) {
    const delta: Record<string, [number, number]> = { ArrowLeft: [-16, 0], ArrowRight: [16, 0], ArrowUp: [0, -16], ArrowDown: [0, 16] }
    const offset = delta[event.key]
    const box = element.value?.getBoundingClientRect()
    if (!floating.value || !offset || !box) return
    event.preventDefault()
    position.value = clamp(box.left + offset[0], box.top + offset[1])
    persist()
  }
  function reset() {
    position.value = null
    try { localStorage.removeItem(STORAGE_KEY) } catch { /* Position is optional. */ }
  }
  onMounted(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null')
      if (saved && Number.isFinite(saved.x) && Number.isFinite(saved.y)) position.value = clamp(saved.x, saved.y)
    } catch { /* Ignore invalid or unavailable storage. */ }
    window.addEventListener('resize', fit)
    observer = new ResizeObserver(fit)
    if (element.value) observer.observe(element.value)
  })
  watch(element, (value, old) => {
    if (old) observer?.unobserve(old)
    if (value) observer?.observe(value)
    void nextTick(fit)
  })
  watch(floating, () => { void nextTick(fit) })
  onBeforeUnmount(() => { observer?.disconnect(); window.removeEventListener('resize', fit) })
  return { style, dragging, start, move, finish, keydown, reset }
}
