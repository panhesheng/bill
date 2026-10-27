import request from './request'

export type BillKind = 'expense' | 'income'
// PersonalBillKind adds transfer (money moved between two accounts of the same
// user) and refund (money returned for a previous expense). Neither is part of
// the income/expense category tree.
export type PersonalBillKind = BillKind | 'transfer' | 'refund'
export type PersonalAccountType = 'bank_card' | 'wechat' | 'alipay' | 'cash'
export type BankCardKind = 'debit' | 'credit'

export interface BillAttachment {
  id?: number
  path: string
  name: string
  sort_order?: number
  created_at?: string
}

export interface PersonalTransaction {
  id: number
  user_id: number
  ledger_id: number
  ledger_name?: string
  account_id: number
  account_name?: string
  account_type?: PersonalAccountType
  counterpart_account_id?: number
  counterpart_account_name?: string
  refund_source_id?: number
  refund_source_category?: string
  refund_source_amount_cents?: number
  refund_source_occurred_at?: string
  kind: PersonalBillKind
  amount_cents: number
  category: string
  note: string
  occurred_at: string
  created_at: string
  updated_at: string
  tags: PersonalTag[]
  attachments?: BillAttachment[]
}

// RefundableTransaction is an expense that still has money left to refund.
export interface RefundableTransaction {
  id: number
  category: string
  note: string
  amount_cents: number
  occurred_at: string
  account_id: number
  account_name: string
  refunded_cents: number
  refundable_cents: number
}

export interface PersonalAccount {
  id: number
  user_id?: number
  type: PersonalAccountType
  name: string
  institution: string
  identifier: string
  card_kind?: BankCardKind
  archived: boolean
  transaction_count?: number
  // sort_order is the position of the account in the account list. The user can
  // drag the accounts into the order they prefer.
  sort_order?: number
  // opening_balance_cents is the balance the account started with. It can only
  // be set when the account is created.
  opening_balance_cents?: number
  // balance_cents is the money currently held by the account.
  balance_cents?: number
  // can_delete tells whether the account may be deleted for good.
  can_delete?: boolean
  created_at?: string
  updated_at?: string
}

export interface PersonalAccountPayload {
  type: PersonalAccountType
  name: string
  institution: string
  identifier: string
  card_kind?: BankCardKind
  card_number?: string
  // opening_balance_cents is only used when creating an account.
  opening_balance_cents?: number
}

export interface PersonalAccountOptions {
  banks: BankOption[]
  card_kinds: Array<{ value: BankCardKind; label: string }>
}

export interface BankOption {
  code: string
  name: string
  short: string
  color: string
}

export interface PersonalLedger {
  id: number
  user_id?: number
  name: string
  currency: string
  cover?: string
  sort_order?: number
  is_default?: boolean
  archived?: boolean
  transaction_count?: number
  // can_delete tells whether the ledger may be deleted for good.
  can_delete?: boolean
  created_at?: string
  updated_at?: string
}

export interface PersonalCategory {
  id: number
  user_id?: number
  name: string
  kind: BillKind
  parent_id: number
  parent_name?: string
  is_default?: boolean
  transaction_count?: number
}

export interface PersonalTag {
  id: number
  user_id?: number
  name: string
  color?: string
  transaction_count?: number
}

export interface StatsSummary {
  income_cents: number
  expense_cents: number
  balance_cents: number
  count: number
}

export interface PersonalStats {
  summary: StatsSummary
  categories: Array<{ category: string; parent?: string; amount_cents: number; count: number }>
  trend: Array<{ month: string; income_cents: number; expense_cents: number }>
  monthly: Array<{ month: string; income_cents: number; expense_cents: number; balance_cents: number; count: number }>
}

// FlowEntry is one period of the cash flow list.
export interface FlowEntry {
  key: string
  title: string
  subtitle: string
  income_cents: number
  expense_cents: number
  balance_cents: number
  count: number
}

export interface DataShareViewer {
  id: number
  user_id: number
  username: string
  created_at: string
}

export interface DataShareOwner {
  user_id: number
  username: string
}

export interface PersonalShares {
  viewers: DataShareViewer[]
  owners: DataShareOwner[]
}

export interface PersonalFlow {
  granularity: 'year' | 'month' | 'day'
  year: number
  month: number
  entries: FlowEntry[]
  available_years: number[]
}

export interface LedgerListItem {
  id: number
  name: string
  currency: string
  owner_user_id: number
  owner_username: string
  archived: boolean
  member_status: 'pending' | 'active' | 'rejected'
  member_role: 'owner' | 'member'
  member_count: number
  balance_cents: number
  expense_cents: number
  updated_at: string
}

export interface LedgerMember {
  id: number
  ledger_id: number
  user_id: number
  username: string
  role: 'owner' | 'member'
  status: 'pending' | 'active' | 'rejected'
  created_at: string
}

export interface SharedShare {
  user_id: number
  username?: string
  amount_cents: number
}

export interface SharedTransaction {
  id: number
  ledger_id: number
  created_by: number
  creator_username: string
  kind: BillKind
  actor_user_id: number
  actor_username: string
  account_id: number
  account_name?: string
  account_type?: PersonalAccountType
  amount_cents: number
  category: string
  note: string
  occurred_at: string
  shares: SharedShare[]
  attachments?: BillAttachment[]
}

export interface Balance {
  user_id: number
  username: string
  balance_cents: number
  paid_cents: number
  share_cents: number
}

export interface Transfer {
  from_user_id: number
  from_username: string
  to_user_id: number
  to_username: string
  amount_cents: number
}

export interface Settlement {
  id: number
  ledger_id: number
  from_user_id: number
  from_username: string
  to_user_id: number
  to_username: string
  amount_cents: number
  note: string
  occurred_at: string
  created_by: number
}

export interface LedgerDetail {
  ledger: {
    id: number
    name: string
    currency: string
    owner_user_id: number
    archived: boolean
    show_account: boolean
    show_category: boolean
    show_note: boolean
    created_at: string
    updated_at: string
  }
  members: LedgerMember[]
  transactions: SharedTransaction[]
  settlements: Settlement[]
  balances: Balance[]
  suggested_transfers: Transfer[] | null
  totals: { expense_cents: number; income_cents: number; expense_count: number; income_count: number }
  transactions_total: number
  transactions_page: number
  transactions_page_size: number
}

export interface PersonalPayload {
  ledger_id: number
  account_id: number
  counterpart_account_id?: number
  refund_source_id?: number
  kind: PersonalBillKind
  amount_cents: number
  category: string
  note: string
  occurred_at: string
  tag_ids: number[]
  attachments?: BillAttachment[]
}

export interface SharedPayload {
  kind: BillKind
  amount_cents: number
  category: string
  note: string
  occurred_at: string
  actor_user_id: number
  account_id?: number
  shares: SharedShare[]
  attachments?: BillAttachment[]
}

export const billingApi = {
  dashboard: (ownerId = 0) => request.get('/api/billing/dashboard', { params: ownerId ? { owner_id: ownerId } : {} }),
  users: (q = '') => request.get('/api/billing/users', { params: { q } }),

  personalList: (params: Record<string, unknown>) => request.get('/api/billing/personal/transactions', { params }),
  personalCreate: (data: PersonalPayload) => request.post('/api/billing/personal/transactions', data),
  personalUpdate: (id: number, data: PersonalPayload) => request.put(`/api/billing/personal/transactions/${id}`, data),
  personalDelete: (id: number) => request.delete(`/api/billing/personal/transactions/${id}`),
  personalRefundableTransactions: (params: { ledger_id?: number; keyword?: string; limit?: number } = {}) => request.get('/api/billing/personal/refundable-transactions', { params }),
  personalStats: (params: Record<string, unknown> = {}) => request.get('/api/billing/personal/stats', { params }),
  personalFlow: (params: { year?: number; month?: number; ledger_id?: number; owner_id?: number } = {}) => request.get('/api/billing/personal/flow', { params }),
  personalExport: (params: Record<string, unknown> = {}) => request.get('/api/billing/personal/export', { params, responseType: 'blob' }),
  personalShares: () => request.get('/api/billing/personal/shares'),
  personalShareCreate: (data: { user_id?: number; username?: string }) => request.post('/api/billing/personal/shares', data),
  personalShareDelete: (id: number) => request.delete(`/api/billing/personal/shares/${id}`),
  personalAccounts: (includeArchived = false, ownerId = 0) => request.get('/api/billing/personal/accounts', { params: { ...(includeArchived ? { include_archived: true } : {}), ...(ownerId ? { owner_id: ownerId } : {}) } }),
  personalAccountOptions: () => request.get('/api/billing/personal/account-options'),
  personalAccountCreate: (data: PersonalAccountPayload) => request.post('/api/billing/personal/accounts', data),
  personalAccountOrder: (accountIds: number[]) => request.put('/api/billing/personal/accounts/order', { account_ids: accountIds }),
  personalAccountUpdate: (id: number, data: PersonalAccountPayload) => request.put(`/api/billing/personal/accounts/${id}`, data),
  personalAccountArchive: (id: number) => request.delete(`/api/billing/personal/accounts/${id}`),
  personalAccountDelete: (id: number) => request.delete(`/api/billing/personal/accounts/${id}/permanent`),
  personalAccountRestore: (id: number) => request.post(`/api/billing/personal/accounts/${id}/restore`),
  personalLedgers: (includeArchived = false, ownerId = 0) => request.get('/api/billing/personal/ledgers', { params: { ...(includeArchived ? { include_archived: true } : {}), ...(ownerId ? { owner_id: ownerId } : {}) } }),
  personalLedgerCreate: (data: { name: string; currency: string; cover?: string }) => request.post('/api/billing/personal/ledgers', data),
  personalLedgerUpdate: (id: number, data: { name: string; currency: string; cover?: string }) => request.put(`/api/billing/personal/ledgers/${id}`, data),
  personalLedgerOrder: (ledgerIds: number[]) => request.put('/api/billing/personal/ledgers/order', { ledger_ids: ledgerIds }),
  personalLedgerDelete: (id: number) => request.delete(`/api/billing/personal/ledgers/${id}`),
  personalLedgerPermanentDelete: (id: number) => request.delete(`/api/billing/personal/ledgers/${id}/permanent`),
  personalLedgerRestore: (id: number) => request.post(`/api/billing/personal/ledgers/${id}/restore`),
  uploadImage: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return request.post('/api/upload', form)
  },
  personalCategories: (ownerId = 0) => request.get('/api/billing/personal/categories', { params: ownerId ? { owner_id: ownerId } : {} }),
  personalCategoryCreate: (data: { name: string; kind: BillKind; parent_id: number }) => request.post('/api/billing/personal/categories', data),
  personalCategoryUpdate: (id: number, data: { name: string; kind: BillKind; parent_id: number }) => request.put(`/api/billing/personal/categories/${id}`, data),
  personalCategoryDelete: (id: number) => request.delete(`/api/billing/personal/categories/${id}`),
  personalTags: (ownerId = 0) => request.get('/api/billing/personal/tags', { params: ownerId ? { owner_id: ownerId } : {} }),
  personalTagCreate: (data: { name: string }) => request.post('/api/billing/personal/tags', data),
  personalTagUpdate: (id: number, data: { name: string }) => request.put(`/api/billing/personal/tags/${id}`, data),
  personalTagDelete: (id: number) => request.delete(`/api/billing/personal/tags/${id}`),

  ledgers: () => request.get('/api/billing/ledgers'),
  ledgerCreate: (data: { name: string; currency: string; usernames: string[] }) => request.post('/api/billing/ledgers', data),
  ledger: (id: number, params: { transaction_page?: number; transaction_page_size?: number } = {}) => request.get(`/api/billing/ledgers/${id}`, { params }),
  ledgerUpdate: (id: number, data: { name: string; currency: string; show_account?: boolean; show_category?: boolean; show_note?: boolean }) => request.put(`/api/billing/ledgers/${id}`, data),
  ledgerArchive: (id: number) => request.delete(`/api/billing/ledgers/${id}`),
  ledgerRestore: (id: number) => request.post(`/api/billing/ledgers/${id}/restore`),
  invite: (id: number, username: string) => request.post(`/api/billing/ledgers/${id}/members`, { username }),
  removeMember: (id: number, userId: number) => request.delete(`/api/billing/ledgers/${id}/members/${userId}`),
  respondInvitation: (id: number, accept: boolean) => request.post(`/api/billing/ledgers/${id}/invitations/respond`, { accept }),

  sharedCreate: (id: number, data: SharedPayload) => request.post(`/api/billing/ledgers/${id}/transactions`, data),
  sharedUpdate: (id: number, transactionId: number, data: SharedPayload) => request.put(`/api/billing/ledgers/${id}/transactions/${transactionId}`, data),
  sharedDelete: (id: number, transactionId: number) => request.delete(`/api/billing/ledgers/${id}/transactions/${transactionId}`),
  settlementCreate: (id: number, data: { from_user_id: number; to_user_id: number; amount_cents: number; note: string; occurred_at: string }) =>
    request.post(`/api/billing/ledgers/${id}/settlements`, data),
  settlementDelete: (id: number, settlementId: number) => request.delete(`/api/billing/ledgers/${id}/settlements/${settlementId}`),
}
