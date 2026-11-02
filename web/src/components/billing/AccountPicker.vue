<template>
  <div>
    <label class="form-field">
      <span>{{ label }}</span>
      <select v-model="selectedID" class="input-field" :aria-label="label" :disabled="loading">
        <option :value="0" disabled>{{ loading ? '加载中' : placeholder }}</option>
        <option v-for="item in accounts" :key="item.id" :value="item.id" :disabled="isDisabled(item.id)">{{ accountOptionLabel(item) }}</option>
      </select>
      <small v-if="hint">{{ hint }}</small>
    </label>
    <div v-if="recentAccounts.length" class="mt-3">
      <p class="text-xs font-semibold text-muted-foreground">最近常用</p>
      <div class="mt-1.5 flex flex-wrap gap-2" aria-label="常用账户">
        <button
          v-for="item in recentAccounts"
          :key="item.id"
          type="button"
          class="min-h-11 touch-manipulation rounded-full border px-3 text-sm transition-colors"
          :class="item.id === modelValue ? 'border-brand-500 bg-brand-100 font-semibold text-brand-800 dark:bg-brand-800 dark:text-accent' : 'border-border text-muted-foreground hover:border-brand-500 hover:text-foreground'"
          @click="pick(item.id)"
        >
          {{ item.name }}
          <span class="ml-1 text-xs opacity-70">{{ item.balance }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDataShareStore } from '../../stores/dataShare'
import { billingApi, type PersonalAccount, type PersonalRecentAccount } from '../../api/billing'
import { accountOptionLabel, formatMoney } from '../../utils/billing'

const props = withDefaults(defineProps<{
  modelValue: number
  accounts: PersonalAccount[]
  label?: string
  placeholder?: string
  hint?: string
  // Accounts that cannot be picked here, e.g. the account a transfer leaves.
  disabledIds?: number[]
  loading?: boolean
}>(), { label: '收支账户', placeholder: '请选择账户', hint: '', disabledIds: () => [], loading: false })

const emit = defineEmits<{ 'update:modelValue': [value: number] }>()
const shareStore = useDataShareStore()
const recent = ref<PersonalRecentAccount[]>([])

const selectedID = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', Number(value)),
})

function isDisabled(id: number) { return props.disabledIds.includes(id) }

// The accounts the user records bills with most often, so a bill can be filed
// with a single click.
const recentAccounts = computed(() => recent.value
  .filter(item => !isDisabled(item.id))
  .map(item => {
    const account = props.accounts.find(entry => entry.id === item.id)
    if (!account) return null
    return { id: account.id, name: account.name, balance: formatMoney(account.balance_cents || 0) }
  })
  .filter((item): item is { id: number; name: string; balance: string } => Boolean(item)))

function pick(id: number) { if (isDisabled(id)) return; emit('update:modelValue', id) }

async function loadRecent() {
  try {
    const response = await billingApi.personalRecentAccounts(shareStore.currentOwnerId)
    recent.value = (response.data?.data?.items || []) as PersonalRecentAccount[]
  } catch {
    recent.value = []
  }
}
watch(() => shareStore.currentOwnerId, loadRecent, { immediate: true })
</script>
