import { describe, expect, it } from 'vitest'
import { moveItem } from './useDragSort'

describe('moveItem', () => {
  const items = [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }]

  it('takes the place of the target item', () => {
    expect(moveItem(items, 1, 3).map(item => item.id)).toEqual([2, 3, 1, 4])
    expect(moveItem(items, 3, 1).map(item => item.id)).toEqual([3, 1, 2, 4])
    expect(moveItem(items, 4, 1).map(item => item.id)).toEqual([4, 1, 2, 3])
    expect(moveItem(items, 1, 4).map(item => item.id)).toEqual([2, 3, 4, 1])
  })

  it('keeps the array untouched when nothing moves', () => {
    expect(moveItem(items, 2, 2)).toBe(items)
    expect(moveItem(items, 2, 99)).toBe(items)
    expect(moveItem(items, 99, 2)).toBe(items)
  })
})
