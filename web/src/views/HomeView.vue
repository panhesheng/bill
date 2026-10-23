<template>
  <div class="page-container animate-fade-in">
    <header class="dashboard-heading">
      <div>
        <div class="flex items-center gap-2">
          <span class="status-dot" aria-hidden="true"></span>
          <p class="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-brand-700 dark:text-brand-300">
            {{ currentMonthText }} · 个人财务
          </p>
        </div>
        <h1 class="mt-3 max-w-2xl font-display text-[clamp(1.75rem,8vw,2.65rem)] font-extrabold leading-[1.12] tracking-[-0.04em]">
          <span class="flex min-w-0 items-baseline">
            <span class="shrink-0">{{ greeting }}，</span>
            <span class="truncate" :title="authStore.user?.username">{{ authStore.user?.username }}</span>
          </span>
          <span class="block text-muted-foreground">钱花在哪，心里有数。</span>
        </h1>
      </div>
      <div class="flex w-full gap-2 sm:w-auto">
        <button v-if="!shareStore.isSharedView" class="btn-brand min-h-12 flex-1 sm:flex-none" @click="showQuickEntry = true">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v12m6-6H6"/></svg>
          记一笔
        </button>
        <RouterLink to="/admin/shared" class="btn-ghost min-h-12 flex-1 sm:flex-none">去共享</RouterLink>
      </div>
    </header>

    <div v-if="loading" class="dashboard-grid" aria-label="正在加载账单总览">
      <div class="balance-card h-64 animate-pulse"></div>
      <div v-for="n in 2" :key="n" class="surface h-40 animate-pulse rounded-[1.25rem]"></div>
      <div class="surface col-span-2 h-72 animate-pulse rounded-[1.25rem]"></div>
    </div>

    <template v-else>
      <div class="dashboard-grid">
        <article :class="{ 'balance-card-empty': isPersonalEmpty }" class="balance-card">
          <div class="relative z-10 flex items-start justify-between gap-4">
            <div>
              <p class="balance-kicker font-mono text-[10px] font-semibold tracking-[0.18em]">本月账本</p>
              <h2 class="balance-title mt-1 text-sm font-bold">本月结余</h2>
            </div>
            <span class="balance-icon flex h-11 w-11 items-center justify-center rounded-full border text-accent">
              <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M5 4h14v16l-3-2-4 2-4-2-3 2V4Zm4 5h6m-6 4h4"/></svg>
            </span>
          </div>
          <strong class="balance-amount relative z-10 mt-8 block font-mono text-[2.25rem] font-semibold leading-none tracking-[-0.05em] tabular-nums sm:text-5xl">
            {{ formatMoney(monthSummary.balance_cents) }}
          </strong>
          <div v-if="isPersonalEmpty" class="relative z-10 mt-auto flex flex-wrap items-center justify-between gap-3 pt-6">
            <div>
              <p class="balance-title text-sm font-bold">从第一笔开始建立你的财务轨迹</p>
              <p class="balance-kicker mt-1 text-xs">记录后，收支趋势和分类统计会自动生成。</p>
            </div>
            <button v-if="!shareStore.isSharedView" class="inline-flex min-h-11 items-center gap-2 rounded-full bg-accent px-4 text-sm font-bold text-brand-900 transition-opacity hover:opacity-90 active:opacity-75" @click="showQuickEntry = true">
              开始记账
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m9 18 6-6-6-6"/></svg>
            </button>
          </div>
          <div v-else class="relative z-10 mt-auto flex flex-wrap items-center gap-2 pt-7">
            <span class="balance-chip">{{ monthSummary.count }} 笔账单</span>
            <span class="balance-chip">储蓄率 {{ savingsRate }}%</span>
          </div>
        </article>

        <article class="cash-card income-card">
          <div class="flex items-start justify-between gap-2">
            <div>
              <p class="cash-kicker">收入</p>
              <h2 class="cash-title">本月收入</h2>
            </div>
            <span class="cash-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19V5m-5 5 5-5 5 5"/></svg>
            </span>
          </div>
          <strong class="cash-value text-emerald-700 dark:text-emerald-300">{{ formatMoney(monthSummary.income_cents) }}</strong>
          <p class="cash-meta">流入账户</p>
        </article>

        <article class="cash-card expense-card">
          <div class="flex items-start justify-between gap-2">
            <div>
              <p class="cash-kicker">支出</p>
              <h2 class="cash-title">本月支出</h2>
            </div>
            <span class="cash-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v14m5-5-5 5-5-5"/></svg>
            </span>
          </div>
          <strong class="cash-value text-[#bb4944] dark:text-[#ff9a90]">{{ formatMoney(monthSummary.expense_cents) }}</strong>
          <p class="cash-meta">已经花掉</p>
        </article>

        <section class="trend-card surface">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="section-kicker">收支流水</p>
              <h2 class="section-title">{{ flowTitle }}</h2>
              <p class="section-copy">收入、支出与结余的逐期对比</p>
            </div>
            <RouterLink to="/admin/analysis" class="section-link">详细分析</RouterLink>
          </div>
          <div class="mt-4 flex flex-wrap items-center gap-2">
            <label class="flow-picker">
              <span class="sr-only">选择年份</span>
              <select v-model.number="flowYear" class="input-field h-10 min-h-10 py-0 text-sm" @change="onFlowYearChange">
                <option :value="0">全部年份</option>
                <option v-for="item in flowYears" :key="item" :value="item">{{ item }} 年</option>
              </select>
            </label>
            <label v-if="flowYear" class="flow-picker">
              <span class="sr-only">选择月份</span>
              <select v-model.number="flowMonth" class="input-field h-10 min-h-10 py-0 text-sm" @change="loadFlow">
                <option :value="0">全部月份</option>
                <option v-for="item in monthOptions" :key="item" :value="item">{{ item }} 月</option>
              </select>
            </label>
            <span class="text-xs text-muted-foreground">{{ flowUnitLabel }}</span>
          </div>
          <div v-if="flowLoading" class="mt-5 space-y-2">
            <div v-for="n in 4" :key="n" class="h-16 animate-pulse rounded-xl bg-muted/60"></div>
          </div>
          <div v-else-if="!flowEntries.length" class="chart-empty">
            <div class="empty-bars" aria-hidden="true"><i></i><i></i><i></i><i></i><i></i><i></i></div>
            <p class="mt-5 font-bold">这段时间还没有收支</p>
            <p class="mt-1 text-sm text-muted-foreground">换个时间范围看看，或先记下第一笔。</p>
          </div>
          <div v-else class="mt-4 divide-y divide-border">
            <article v-for="entry in flowEntries" :key="entry.key" class="flex flex-wrap items-center gap-x-4 gap-y-1 py-3">
              <div class="min-w-0 flex-1 basis-24">
                <p class="truncate font-display text-lg font-bold leading-tight">{{ entry.title }}</p>
                <p v-if="entry.subtitle" class="font-mono text-[11px] text-muted-foreground">{{ entry.subtitle }}</p>
              </div>
              <div class="flex min-w-0 flex-[2] flex-wrap items-baseline justify-end gap-x-4 gap-y-1 text-sm">
                <span class="text-muted-foreground">收入 <strong class="font-mono font-semibold tabular-nums text-brand-700 dark:text-brand-300">{{ formatMoney(entry.income_cents) }}</strong></span>
                <span class="text-muted-foreground">支出 <strong class="font-mono font-semibold tabular-nums text-[#bb4944] dark:text-[#ff9a90]">{{ formatMoney(entry.expense_cents) }}</strong></span>
              </div>
              <div class="w-full text-right sm:w-auto sm:min-w-36">
                <span class="text-xs text-muted-foreground">结余</span>
                <strong :class="entry.balance_cents >= 0 ? 'text-foreground' : 'text-[#bb4944] dark:text-[#ff9a90]'" class="ml-2 font-mono text-base font-bold tabular-nums">{{ formatMoney(entry.balance_cents) }}</strong>
              </div>
            </article>
          </div>
        </section>

        <section class="recent-card surface">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="section-kicker">账单动态</p>
              <h2 class="section-title">最近账单</h2>
            </div>
            <RouterLink to="/admin/personal" class="section-link">全部</RouterLink>
          </div>
          <div v-if="!recent.length" class="empty-state h-52">
            <span class="empty-receipt" aria-hidden="true"></span>
            <p class="mt-4 font-bold text-foreground">账本还是空的</p>
            <p class="mt-1 text-center">先记录一笔，开始了解自己的钱。</p>
          </div>
          <div v-else class="mt-4 divide-y divide-border">
            <div v-for="item in recent.slice(0, 5)" :key="item.id" class="flex items-center gap-3 py-3">
              <div :class="recentIconClass(item)" class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full">
                <CategoryIcon :category="item.category" :kind="item.kind" class="h-4 w-4" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-bold">{{ recentTitle(item) }}<span v-if="item.note" class="font-normal text-muted-foreground"> · {{ item.note }}</span></div>
                <div class="font-mono text-[10px] text-muted-foreground">{{ formatDate(item.occurred_at) }}</div>
              </div>
              <div :class="recentAmountClass(item)" class="font-mono text-xs font-semibold tabular-nums sm:text-sm">
                {{ recentAmountPrefix(item) }}{{ formatMoney(item.amount_cents) }}
              </div>
            </div>
          </div>
        </section>

        <section class="shared-card surface">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="section-kicker">共同记账</p>
              <h2 class="section-title">共享账本</h2>
              <p class="section-copy">一起花的钱，也能清清楚楚。</p>
            </div>
            <span v-if="pendingInvitations" class="badge bg-accent text-brand-900">{{ pendingInvitations }} 个待处理邀请</span>
          </div>
          <div v-if="activeLedgers.length" class="settlement-strip">
            <div class="settlement-copy">
              <span class="settlement-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M4 7h16v12H4zM8 7V5h8v2m-8 6h8"/></svg>
              </span>
              <div>
                <p class="text-sm font-extrabold">我的待结算</p>
                <p class="mt-0.5 text-xs text-muted-foreground">
                  <template v-if="sharedSettlement.singleCurrency">
                    应收 {{ formatMoney(sharedSettlement.receivable, sharedSettlement.currency) }} ·
                    应付 {{ formatMoney(sharedSettlement.payable, sharedSettlement.currency) }}
                  </template>
                  <template v-else>{{ sharedSettlement.unsettledCount }} 个账本存在待结算款项</template>
                </p>
              </div>
            </div>
            <RouterLink
              :to="sharedSettlement.targetId ? `/admin/shared/${sharedSettlement.targetId}` : '/admin/shared'"
              class="btn-ghost min-h-11 shrink-0"
            >
              {{ sharedSettlement.unsettledCount ? '去结算' : '查看账本' }}
            </RouterLink>
          </div>
          <div v-if="!ledgers.length" class="shared-empty">
            <div>
              <p class="font-bold">还没有共享账本</p>
              <p class="mt-1 text-sm text-muted-foreground">旅行、合租或家庭开支，从创建账本开始。</p>
            </div>
            <RouterLink to="/admin/shared" class="btn-ghost min-h-11">创建账本</RouterLink>
          </div>
          <div v-else class="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <RouterLink v-for="ledger in ledgers.slice(0, 6)" :key="ledger.id" :to="ledger.member_status === 'active' ? `/admin/shared/${ledger.id}` : '/admin/shared'" class="ledger-tile group">
              <div class="flex items-start justify-between gap-3">
                <span class="ledger-index">{{ String(ledger.id).padStart(2, '0') }}</span>
                <span v-if="ledger.member_status === 'pending'" class="badge bg-accent text-brand-900">待接受</span>
                <svg v-else class="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="m9 18 6-6-6-6"/></svg>
              </div>
              <h3 class="mt-5 font-display text-lg font-extrabold">{{ ledger.name }}</h3>
              <p class="mt-1 text-xs text-muted-foreground">{{ ledger.member_count }} 位成员 · {{ ledger.owner_username }}</p>
              <div v-if="ledger.member_status === 'active'" class="mt-5 flex items-end justify-between border-t border-border pt-3">
                <span class="text-xs text-muted-foreground">{{ ledger.balance_cents > 0 ? '应收' : ledger.balance_cents < 0 ? '应付' : '已结清' }}</span>
                <strong :class="ledger.balance_cents > 0 ? 'text-brand-700 dark:text-brand-300' : ledger.balance_cents < 0 ? 'text-[#bb4944] dark:text-[#ff9a90]' : 'text-foreground'" class="font-mono text-base tabular-nums">{{ formatMoney(Math.abs(ledger.balance_cents), ledger.currency) }}</strong>
              </div>
            </RouterLink>
          </div>
        </section>
      </div>
    </template>

    <QuickPersonalEntryModal v-model="showQuickEntry" @saved="handleQuickEntrySaved" />
    <Toast :message="toast.message" :type="toast.type" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { billingApi, type FlowEntry, type LedgerListItem, type PersonalTransaction, type StatsSummary } from '../api/billing'
import { useAuthStore } from '../stores/auth'
import { useDataShareStore } from '../stores/dataShare'
import { apiMessage, formatDate, formatMoney } from '../utils/billing'
import CategoryIcon from '../components/billing/CategoryIcon.vue'
import QuickPersonalEntryModal from '../components/billing/QuickPersonalEntryModal.vue'
import Toast from '../components/Toast.vue'

const authStore = useAuthStore()
const shareStore = useDataShareStore()
const loading = ref(true)
const showQuickEntry = ref(false)
const toast = reactive<{ message: string; type: 'success' | 'error' }>({ message: '', type: 'success' })
const monthSummary = ref<StatsSummary>({ income_cents: 0, expense_cents: 0, balance_cents: 0, count: 0 })
const flowEntries = ref<FlowEntry[]>([])
const flowYears = ref<number[]>([])
const flowYear = ref(new Date().getFullYear())
const flowMonth = ref(0)
const flowLoading = ref(false)
const recent = ref<PersonalTransaction[]>([])
const ledgers = ref<LedgerListItem[]>([])
const pendingInvitations = ref(0)
const activeLedgers = computed(() => ledgers.value.filter(item => item.member_status === 'active' && !item.archived))
const isPersonalEmpty = computed(() => monthSummary.value.count === 0 && recent.value.length === 0)
const sharedSettlement = computed(() => {
  const items = activeLedgers.value
  const currencies = new Set(items.map(item => item.currency))
  const unsettled = items.filter(item => item.balance_cents !== 0)
  return {
    receivable: items.reduce((sum, item) => sum + Math.max(0, item.balance_cents), 0),
    payable: items.reduce((sum, item) => sum + Math.max(0, -item.balance_cents), 0),
    unsettledCount: unsettled.length,
    targetId: unsettled[0]?.id || items[0]?.id,
    singleCurrency: currencies.size <= 1,
    currency: items[0]?.currency || 'CNY',
  }
})

const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 6) return '夜深了'
  if (hour < 12) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const currentMonthText = computed(() => new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit' }).format(new Date()).replace('/', '.'))
const monthOptions = computed(() => Array.from({ length: 12 }, (_, index) => index + 1))
const flowTitle = computed(() => {
  if (!flowYear.value) return '全部收支流水'
  if (flowMonth.value) return `${flowYear.value} 年 ${flowMonth.value} 月`
  return `${flowYear.value} 年收支`
})
const flowUnitLabel = computed(() => {
  if (!flowYear.value) return '按年份汇总，倒序排列'
  if (flowMonth.value) return '按日汇总，倒序排列'
  return '按月汇总，倒序排列'
})
const savingsRate = computed(() => {
  if (monthSummary.value.income_cents <= 0) return 0
  return Math.round(monthSummary.value.balance_cents / monthSummary.value.income_cents * 100)
})
async function loadDashboard() {
  loading.value = true
  try {
    const response = await billingApi.dashboard(shareStore.currentOwnerId)
    const data = response.data.data
    monthSummary.value = data.month.summary
    recent.value = data.recent || []
    ledgers.value = data.ledgers || []
    pendingInvitations.value = data.pending_invitations || 0
  } catch (error) {
    notify(apiMessage(error, '首页数据加载失败'), 'error')
  } finally {
    loading.value = false
  }
  loadFlow()
}

async function loadFlow() {
  flowLoading.value = true
  try {
    const response = await billingApi.personalFlow({ year: flowYear.value, month: flowMonth.value, ...shareStore.ownerParam })
    const data = response.data?.data
    flowEntries.value = data?.entries || []
    const years = data?.available_years || []
    const current = new Date().getFullYear()
    // The current year stays selectable even before the first bill is written.
    flowYears.value = years.includes(current) ? years : [current, ...years]
  } catch (error) {
    notify(apiMessage(error, '收支流水加载失败'), 'error')
  } finally {
    flowLoading.value = false
  }
}

function onFlowYearChange() {
  // Switching to "all years" keeps the month out of the request.
  flowMonth.value = 0
  loadFlow()
}

function notify(message: string, type: 'success' | 'error' = 'success') {
  toast.message = ''
  setTimeout(() => {
    toast.message = message
    toast.type = type
  })
}

function handleQuickEntrySaved() {
  notify('账单已记录')
  loadDashboard()
}

function recentIconClass(item: PersonalTransaction) {
  if (item.kind === 'transfer') return 'bg-sky-500/10 text-sky-700 dark:text-sky-300'
  if (item.kind === 'refund') return 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
  return item.kind === 'income' ? 'bg-brand-100 text-brand-700 dark:bg-brand-700/40 dark:text-brand-300' : 'bg-[#f8ded8] text-[#a83b36] dark:bg-[#7d322f]/35 dark:text-[#ff9a90]'
}
function recentAmountClass(item: PersonalTransaction) {
  if (item.kind === 'transfer') return 'text-sky-700 dark:text-sky-300'
  if (item.kind === 'refund') return 'text-amber-700 dark:text-amber-300'
  return item.kind === 'income' ? 'text-brand-700 dark:text-brand-300' : 'text-foreground'
}
function recentAmountPrefix(item: PersonalTransaction) {
  if (item.kind === 'transfer' || item.kind === 'refund') return ''
  return item.kind === 'income' ? '+' : '-'
}
function recentTitle(item: PersonalTransaction) {
  if (item.kind === 'transfer') {
    const from = item.account_name || '未设置账户'
    const to = item.counterpart_account_name || '未设置账户'
    return `${from} → ${to}`
  }
  if (item.kind === 'refund') return item.refund_source_category ? `退款 · ${item.refund_source_category}` : item.category
  return item.category
}

watch(() => shareStore.currentOwnerId, () => { loadDashboard(); loadFlow() })
onMounted(loadDashboard)
</script>

<style scoped>
.dashboard-heading { @apply flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between sm:gap-5; }
.status-dot { @apply h-2.5 w-2.5 rounded-full bg-accent shadow-[0_0_0_4px_rgb(var(--color-accent)/.2)]; }
.dashboard-grid { @apply grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4 xl:grid-cols-6; }
.balance-card {
  @apply surface relative col-span-2 flex min-h-[13rem] flex-col overflow-hidden rounded-[1.5rem] p-4 sm:min-h-[16rem] sm:rounded-[1.75rem] sm:p-7 lg:col-span-2 xl:col-span-4;
}
.balance-card-empty { min-height: 14rem; }
.balance-kicker { color: rgb(var(--color-balance-muted)); }
.balance-title, .balance-amount { color: rgb(var(--color-balance-foreground)); }
.balance-icon { border-color: rgb(var(--color-balance-foreground) / .15); background-color: rgb(var(--color-balance-foreground) / .07); }
.balance-chip { @apply rounded-full border px-3 py-1.5 font-mono text-[10px]; border-color: rgb(var(--color-balance-foreground) / .15); background-color: rgb(var(--color-balance-foreground) / .07); color: rgb(var(--color-balance-muted)); }
.cash-card { @apply surface col-span-1 flex min-h-[9rem] flex-col rounded-[1.25rem] p-3 sm:min-h-[12rem] sm:p-5 xl:col-span-1; }
.income-card { @apply border-brand-200 dark:border-brand-700; }
.expense-card { border-color: rgb(234 118 111 / .4); }
.cash-kicker, .section-kicker { @apply font-mono text-[9px] font-semibold uppercase tracking-[0.18em] text-muted-foreground; }
.cash-title { @apply mt-1 whitespace-nowrap text-xs font-extrabold 2xl:text-sm; }
.cash-icon { @apply flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted 2xl:h-9 2xl:w-9; }
.cash-icon svg { @apply h-4 w-4; }
.cash-value { @apply mt-auto break-words pt-4 font-mono text-base font-semibold tracking-[-0.04em] tabular-nums sm:text-xl; }
.cash-meta { @apply mt-1 text-[11px] text-muted-foreground; }
.trend-card { @apply col-span-2 rounded-[1.5rem] p-4 sm:p-6 lg:col-span-2 xl:col-span-3; }
.recent-card { @apply col-span-2 rounded-[1.5rem] p-4 sm:p-6 lg:col-span-2 xl:col-span-3; }
.shared-card { @apply col-span-2 rounded-[1.5rem] p-4 sm:p-6 lg:col-span-4 xl:col-span-6; }
.section-title { @apply mt-1 font-display text-xl font-extrabold tracking-tight; }
.section-copy { @apply mt-1 text-sm text-muted-foreground; }
.section-link { @apply inline-flex min-h-11 items-center rounded-full px-3 text-sm font-bold text-brand-700 transition-colors hover:bg-brand-50 dark:text-brand-300 dark:hover:bg-muted; }
.chart-empty { @apply flex min-h-[13rem] flex-col items-center justify-center; }
.flow-picker { @apply inline-flex; }
.flow-picker select { @apply min-w-28; }
.empty-bars { @apply flex h-14 items-end gap-2; }
.empty-bars i { @apply w-2 rounded-full bg-muted; }
.empty-bars i:nth-child(1), .empty-bars i:nth-child(6) { height: 35%; }
.empty-bars i:nth-child(2), .empty-bars i:nth-child(5) { height: 58%; }
.empty-bars i:nth-child(3), .empty-bars i:nth-child(4) { height: 85%; }
.empty-receipt { @apply block h-12 w-10 rounded-t-lg border-2 border-dashed border-muted-foreground/35; }
.shared-empty { @apply mt-5 flex flex-col gap-4 rounded-[1.25rem] border border-dashed border-border bg-muted/35 p-5 sm:flex-row sm:items-center sm:justify-between; }
.settlement-strip { @apply mt-5 flex flex-col gap-4 rounded-[1.25rem] border border-border bg-muted/35 p-4 sm:flex-row sm:items-center sm:justify-between; }
.settlement-copy { @apply flex min-w-0 items-center gap-3; }
.settlement-icon { @apply flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-brand-100 text-brand-700 dark:bg-brand-700/35 dark:text-brand-300; }
.settlement-icon svg { @apply h-5 w-5; }
.ledger-tile { @apply rounded-[1.25rem] border border-border bg-muted/30 p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-brand-400 hover:bg-surface hover:shadow-soft; }
.ledger-index { @apply font-mono text-xs font-semibold text-muted-foreground; }

@media (min-width: 768px) {
  .balance-card { min-height: 17rem; }
}
</style>
