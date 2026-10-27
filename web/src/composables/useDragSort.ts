import { ref } from 'vue'

/** 可以被拖动排序的条目。 */
export interface DraggableItem {
  id: number
}

/**
 * 把 sourceId 移动到 targetId 原来的位置，返回新的顺序。
 * 源条目与目标条目相同或不存在时返回原数组。
 */
export function moveItem<T extends DraggableItem>(items: T[], sourceId: number, targetId: number): T[] {
  const sourceIndex = items.findIndex(item => item.id === sourceId)
  const targetIndex = items.findIndex(item => item.id === targetId)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return items
  const next = [...items]
  const [moved] = next.splice(sourceIndex, 1)
  next.splice(targetIndex, 0, moved)
  return next
}

export interface DragSortOptions<T extends DraggableItem> {
  /** 当前显示的条目，拖动结束时用它计算新的顺序。 */
  items: () => T[]
  /** 顺序变化后调用，把新顺序保存下来。 */
  onDrop: (items: T[]) => void | Promise<void>
  /** 命中测试用的属性名，默认 data-drag-id。 */
  attribute?: string
}

/**
 * 拖动排序，鼠标和触摸都能用。
 *
 * 原生 HTML5 拖放在移动端不会触发，所以这里用 Pointer Events 实现：
 * 按住手柄后指针移动，被拖动的条目高亮，移到目标条目上松手即完成换位。
 * 目标条目通过 elementFromPoint 命中，因此卡片是几列的栅格布局都能正确工作。
 * 指针靠近屏幕上下边缘时页面会自动滚动，方便把条目拖到远处。
 */
export function useDragSort<T extends DraggableItem>(options: DragSortOptions<T>) {
  const draggingId = ref<number | null>(null)
  const dropTargetId = ref<number | null>(null)
  const attribute = options.attribute || 'data-drag-id'
  let pointerY = 0
  let autoScrollFrame = 0

  function stopAutoScroll() {
    if (autoScrollFrame) cancelAnimationFrame(autoScrollFrame)
    autoScrollFrame = 0
  }
  function autoScroll() {
    const margin = 96
    const bottom = window.innerHeight - margin
    if (pointerY < margin) window.scrollBy(0, -Math.ceil((margin - pointerY) / 8))
    else if (pointerY > bottom) window.scrollBy(0, Math.ceil((pointerY - bottom) / 8))
    autoScrollFrame = requestAnimationFrame(autoScroll)
  }
  function onPointerMove(event: PointerEvent) {
    if (draggingId.value === null) return
    pointerY = event.clientY
    if (event.cancelable) event.preventDefault()
    const element = document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null
    const item = element?.closest?.(`[${attribute}]`) as HTMLElement | null
    const id = item ? Number(item.getAttribute(attribute)) : Number.NaN
    dropTargetId.value = Number.isFinite(id) && id !== draggingId.value ? id : null
  }
  function onPointerUp() {
    const sourceId = draggingId.value
    const targetId = dropTargetId.value
    const current = options.items()
    stop()
    if (sourceId === null || targetId === null) return
    const next = moveItem(current, sourceId, targetId)
    if (next === current) return
    void options.onDrop(next)
  }
  function stop() {
    draggingId.value = null
    dropTargetId.value = null
    stopAutoScroll()
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
  }
  function startDrag(event: PointerEvent, id: number) {
    if (event.button !== 0) return
    draggingId.value = id
    dropTargetId.value = null
    pointerY = event.clientY
    window.addEventListener('pointermove', onPointerMove, { passive: false })
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
    stopAutoScroll()
    autoScrollFrame = requestAnimationFrame(autoScroll)
  }
  return { draggingId, dropTargetId, startDrag, cancelDrag: stop }
}
