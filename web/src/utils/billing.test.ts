import { describe, expect, it } from 'vitest'
import { accountOptionLabel, centsToMoney, formatTime, groupTransactionsByDay, isSplitValid, moneyToCents, pickDefaultAccountId, refundCategory, splitByWeights, splitEqually, splitExactly, transferCategory, validateRefundAmount, validateTransferAccounts } from './billing'

describe('money conversion', () => {
  it('converts decimal money without floating point leakage', () => {
    expect(moneyToCents('12.345')).toBe(1235)
    expect(moneyToCents('not money')).toBe(0)
    expect(centsToMoney(1235)).toBe('12.35')
  })
})

describe('shared bill splits', () => {
  const ids = [11, 22, 33]

  it('splits equally and assigns remainder cents predictably', () => {
    expect(splitEqually(100, ids)).toEqual([
      { user_id: 11, amount_cents: 34 }, { user_id: 22, amount_cents: 33 }, { user_id: 33, amount_cents: 33 },
    ])
  })

  it('uses exact entered amounts', () => {
    const values = { 11: '1.20', 22: '2.30', 33: '0.50' }
    expect(splitExactly(ids, values)).toEqual([
      { user_id: 11, amount_cents: 120 }, { user_id: 22, amount_cents: 230 }, { user_id: 33, amount_cents: 50 },
    ])
    expect(isSplitValid('exact', 400, ids, values)).toBe(true)
    expect(isSplitValid('exact', 1, ids, { 11: '0.005', 22: '0.005', 33: '0' })).toBe(false)
    expect(isSplitValid('exact', 2, ids, { 11: '0.005', 22: '0.005', 33: '0' })).toBe(true)
  })

  it('apportions percentage weights and preserves every cent', () => {
    const result = splitByWeights(101, ids, { 11: 50, 22: 30, 33: 20 })
    expect(result).toEqual([
      { user_id: 11, amount_cents: 50 }, { user_id: 22, amount_cents: 30 }, { user_id: 33, amount_cents: 21 },
    ])
    expect(isSplitValid('percent', 101, ids, { 11: 50, 22: 30, 33: 20 })).toBe(true)
  })

  it('apportions share weights and rejects zero shares', () => {
    expect(splitByWeights(1000, ids, { 11: 1, 22: 2, 33: 1 })).toEqual([
      { user_id: 11, amount_cents: 250 }, { user_id: 22, amount_cents: 500 }, { user_id: 33, amount_cents: 250 },
    ])
    expect(isSplitValid('shares', 1000, ids, { 11: 0, 22: 0, 33: 0 })).toBe(false)
  })

  it('rejects negative and non-finite split values consistently', () => {
    expect(isSplitValid('percent', 100, ids, { 11: -10, 22: 110, 33: 0 })).toBe(false)
    expect(isSplitValid('shares', 100, ids, { 11: -1, 22: 2, 33: 0 })).toBe(false)
    expect(isSplitValid('shares', 100, ids, { 11: Infinity, 22: 1, 33: 1 })).toBe(false)
    expect(isSplitValid('percent', 100, ids, { 11: 'not-a-number', 22: 100, 33: 0 })).toBe(false)
    expect(splitByWeights(100, ids, { 11: -1, 22: 2, 33: 0 })).toEqual([])
    expect(splitExactly(ids, { 11: '-1', 22: '2', 33: '0' })).toEqual([])
  })
})

describe('default account selection', () => {
  const accounts = [{ id: 1 }, { id: 2 }, { id: 3 }]

  it('prefers the last used account', () => {
    expect(pickDefaultAccountId(accounts, [2, 3])).toBe(2)
  })

  it('falls back to the next candidate when the remembered account no longer exists', () => {
    expect(pickDefaultAccountId(accounts, [99, 3])).toBe(3)
  })

  it('falls back to the first account when nothing matches', () => {
    expect(pickDefaultAccountId(accounts, [99])).toBe(1)
    expect(pickDefaultAccountId([], [2])).toBe(0)
  })
})

describe('transfer account validation', () => {
  it('accepts two distinct accounts', () => {
    expect(validateTransferAccounts(1, 2)).toBe('')
  })

  it('requires both accounts', () => {
    expect(validateTransferAccounts(0, 2)).toBe('请选择转出账户')
    expect(validateTransferAccounts(1, 0)).toBe('请选择转入账户')
  })

  it('rejects transferring to the same account', () => {
    expect(validateTransferAccounts(3, 3)).toBe('转出账户与转入账户不能相同')
  })

  it('uses a fixed transfer category', () => {
    expect(transferCategory).toBe('转账')
  })
})

describe('refund category', () => {
  it('uses a fixed refund category', () => {
    expect(refundCategory).toBe('退款')
  })
})

describe('account option label', () => {
  it('shows the name, the identifier and the balance', () => {
    expect(accountOptionLabel({ name: '工资卡', identifier: '1234', balance_cents: 120000 })).toBe('工资卡 · 1234 · 余额 ¥1,200.00')
  })

  it('leaves out the identifier when there is none', () => {
    expect(accountOptionLabel({ name: '日常微信', identifier: '', balance_cents: 5600 })).toBe('日常微信 · 余额 ¥56.00')
  })

  it('treats a missing balance as zero', () => {
    expect(accountOptionLabel({ name: '现金钱包' })).toBe('现金钱包 · 余额 ¥0.00')
  })
})

describe('refund amount validation', () => {
  it('accepts a partial refund', () => {
    expect(validateRefundAmount(3000, 10000)).toBe('')
  })

  it('accepts refunding the whole remaining amount', () => {
    expect(validateRefundAmount(10000, 10000)).toBe('')
  })

  it('rejects an amount above the remaining refundable amount', () => {
    expect(validateRefundAmount(10001, 10000)).toBe('退款金额不能超过剩余可退金额')
  })

  it('rejects a non positive amount', () => {
    expect(validateRefundAmount(0, 10000)).toBe('请输入大于 0 的金额')
    expect(validateRefundAmount(-100, 10000)).toBe('请输入大于 0 的金额')
  })
})

describe('bills grouped by day', () => {
  const now = new Date('2026-11-02T10:00:00')

  it('puts the bills of the same day into one group', () => {
    const groups = groupTransactionsByDay([
      { id: 1, occurred_at: '2026-11-02T08:30:00' },
      { id: 2, occurred_at: '2026-11-02T20:15:00' },
      { id: 3, occurred_at: '2026-11-01T12:00:00' },
    ], now)
    expect(groups.map(group => group.key)).toEqual(['2026-11-02', '2026-11-01'])
    expect(groups[0].items.map(item => item.id)).toEqual([1, 2])
    expect(groups[1].items.map(item => item.id)).toEqual([3])
  })

  it('labels every group with the date and the weekday', () => {
    const groups = groupTransactionsByDay([{ id: 1, occurred_at: '2026-11-02T08:30:00' }], now)
    expect(groups[0].label).toBe('11月2日 周一')
    expect(groups[0].relative).toBe('今天')
  })

  it('marks yesterday and keeps other years visible', () => {
    const groups = groupTransactionsByDay([
      { id: 1, occurred_at: '2026-11-01T08:30:00' },
      { id: 2, occurred_at: '2025-12-31T08:30:00' },
    ], now)
    expect(groups[0].relative).toBe('昨天')
    expect(groups[1].relative).toBe('')
    expect(groups[1].label.startsWith('2025年')).toBe(true)
  })

  it('skips bills without a valid time', () => {
    const groups = groupTransactionsByDay([{ id: 1, occurred_at: 'not a date' }], now)
    expect(groups).toEqual([])
  })

  it('shows the time of a bill without the date', () => {
    expect(formatTime('2026-11-02T08:30:00')).toBe('08:30')
    expect(formatTime('')).toBe('')
  })
})
