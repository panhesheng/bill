export const personalExpenseCategories = ['餐饮', '交通', '购物', '居住', '娱乐', '医疗', '教育', '人情', '旅行', '其他']
export const personalIncomeCategories = ['工资', '奖金', '兼职', '理财', '报销', '礼金', '退款', '其他']
export const sharedExpenseCategories = ['聚餐', '住宿', '交通', '门票', '采购', '房租', '水电', '活动', '其他']
export const sharedIncomeCategories = ['退款', '报销', '补贴', '共同收入', '其他']

export function formatMoney(cents: number, currency = 'CNY') {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency,
    minimumFractionDigits: 2,
  }).format((cents || 0) / 100)
}

export function moneyToCents(value: string | number) {
  const numberValue = typeof value === 'number' ? value : Number.parseFloat(value)
  if (!Number.isFinite(numberValue)) return 0
  return Math.round(numberValue * 100)
}

export type SplitMode = 'equal' | 'exact' | 'percent' | 'shares'

export interface SplitShare {
  user_id: number
  amount_cents: number
}

function splitNumber(value: string | number) {
  if (typeof value === 'string' && value.trim() === '') return 0
  return Number(value)
}

/** Split an integer number of cents evenly, assigning unavoidable remainder cents in input order. */
export function splitEqually(amountCents: number, userIDs: number[]): SplitShare[] {
  if (amountCents <= 0 || !userIDs.length) return []
  const base = Math.floor(amountCents / userIDs.length)
  const remainder = amountCents % userIDs.length
  return userIDs.map((userID, index) => ({ user_id: userID, amount_cents: base + (index < remainder ? 1 : 0) }))
}

/** Allocate cents proportionally and give the final participant any rounding remainder. */
export function splitByWeights(amountCents: number, userIDs: number[], weights: Record<number, string | number>): SplitShare[] {
  if (amountCents <= 0 || !userIDs.length) return []
  const values = userIDs.map(id => splitNumber(weights[id]))
  if (values.some(value => !Number.isFinite(value) || value < 0)) return []
  const total = values.reduce((sum, value) => sum + value, 0)
  if (total <= 0) return []
  let allocated = 0
  return userIDs.map((userID, index) => {
    const amount_cents = index === userIDs.length - 1 ? amountCents - allocated : Math.floor(amountCents * values[index] / total)
    allocated += amount_cents
    return { user_id: userID, amount_cents }
  })
}

export function splitExactly(userIDs: number[], amounts: Record<number, string | number>): SplitShare[] {
  const values = userIDs.map(userID => splitNumber(amounts[userID]))
  if (values.some(value => !Number.isFinite(value) || value < 0)) return []
  return userIDs.map(userID => ({ user_id: userID, amount_cents: moneyToCents(amounts[userID]) }))
}

export function isSplitValid(mode: SplitMode, amountCents: number, userIDs: number[], values: Record<number, string | number>) {
  if (amountCents <= 0 || !userIDs.length) return false
  if (mode === 'equal') return true
  const normalized = userIDs.map(userID => splitNumber(values[userID]))
  if (normalized.some(value => !Number.isFinite(value) || value < 0)) return false
  const total = normalized.reduce((sum, value) => sum + value, 0)
  if (mode === 'exact') return normalized.reduce((sum, value) => sum + moneyToCents(value), 0) === amountCents
  if (mode === 'percent') return Math.abs(total - 100) < 0.001
  return total > 0
}

export function centsToMoney(cents: number) {
  return (cents / 100).toFixed(2)
}

export function toLocalInput(value?: string | Date) {
  const date = value ? new Date(value) : new Date()
  const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return shifted.toISOString().slice(0, 16)
}

export function formatDate(value: string) {
  if (!value) return ''
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

export function apiMessage(error: any, fallback = '操作失败') {
  return error?.response?.data?.message || fallback
}

const LAST_ACCOUNT_STORAGE_KEY = 'bill-last-account-id'

/** 读取上次记账时使用的收支账户 id，没有记录时返回 0。 */
export function readLastAccountId(): number {
  if (typeof localStorage === 'undefined') return 0
  const id = Number.parseInt(localStorage.getItem(LAST_ACCOUNT_STORAGE_KEY) || '', 10)
  return Number.isInteger(id) && id > 0 ? id : 0
}

/** 记住本次使用的收支账户，下次记账时默认选中。 */
export function rememberAccountId(id: number) {
  if (typeof localStorage === 'undefined') return
  if (!Number.isInteger(id) || id <= 0) return
  localStorage.setItem(LAST_ACCOUNT_STORAGE_KEY, String(id))
}

/**
 * 计算新建账单时的默认收支账户：
 * 依次尝试候选值（上次使用的账户、当前筛选的账户），都不存在时回退到第一个账户。
 */
export function pickDefaultAccountId(accounts: Array<{ id: number }>, candidates: number[] = []): number {
  const ids = accounts.map(item => item.id)
  for (const candidate of candidates) {
    if (candidate && ids.includes(candidate)) return candidate
  }
  return accounts[0]?.id || 0
}

/** 转账固定使用的分类，不参与用户的收支分类树。 */
export const transferCategory = '转账'

/** 退款固定使用的分类，不参与用户的收支分类树。 */
export const refundCategory = '退款'

/**
 * 校验转账的转出与转入账户。
 * 返回空字符串表示校验通过，否则返回错误提示。
 */
export function validateTransferAccounts(fromAccountId: number, toAccountId: number): string {
  if (!fromAccountId) return '请选择转出账户'
  if (!toAccountId) return '请选择转入账户'
  if (fromAccountId === toAccountId) return '转出账户与转入账户不能相同'
  return ''
}

/**
 * 校验退款金额。退款可以退全部或退部分，但不能超过原支出的剩余可退金额。
 * 返回空字符串表示校验通过，否则返回错误提示。
 */
export function validateRefundAmount(amountCents: number, refundableCents: number): string {
  if (amountCents <= 0) return '请输入大于 0 的金额'
  if (refundableCents > 0 && amountCents > refundableCents) return '退款金额不能超过剩余可退金额'
  return ''
}
