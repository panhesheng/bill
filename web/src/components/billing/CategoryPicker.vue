<template>
  <div>
    <label class="form-field">
      <span>分类</span>
      <div class="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2">
        <select v-model.number="rootID" class="input-field" aria-label="一级分类" @change="selectRoot">
          <option :value="0" disabled>请选择一级分类</option>
          <option v-for="item in roots" :key="item.id" :value="item.id">{{ item.name }}</option>
          <option v-if="legacy" :value="-1">{{ legacy }}（历史分类）</option>
        </select>
        <svg class="h-4 w-4 shrink-0 text-muted-foreground" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m9 18 6-6-6-6" /></svg>
        <select v-model.number="childID" class="input-field" aria-label="二级分类" :disabled="!children.length" @change="selectChild">
          <option :value="0" disabled>{{ childPlaceholder }}</option>
          <option v-for="item in children" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
      </div>
      <small v-if="hint">{{ hint }}</small>
    </label>
    <div v-if="recent.length" class="mt-3">
      <p class="text-xs font-semibold text-muted-foreground">最近常用</p>
      <div class="mt-1.5 flex flex-wrap gap-2" aria-label="常用分类">
        <button v-for="item in recent" :key="`${item.kind}-${item.name}`" type="button" class="min-h-11 touch-manipulation rounded-full border px-3 text-sm transition-colors" :class="item.name === modelValue ? 'border-brand-500 bg-brand-100 font-semibold text-brand-800 dark:bg-brand-800 dark:text-accent' : 'border-border text-muted-foreground hover:border-brand-500 hover:text-foreground'" @click="pick(item.name)">
          {{ item.name }}
          <span class="ml-1 text-xs opacity-70">{{ item.count }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDataShareStore } from '../../stores/dataShare'
import { billingApi, type BillKind, type PersonalCategory, type PersonalRecentCategory } from '../../api/billing'

const props = defineProps<{ modelValue: string; kind: BillKind; categories: PersonalCategory[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const shareStore = useDataShareStore()
const rootID = ref(0), childID = ref(0)
const recent = ref<PersonalRecentCategory[]>([])
const kindCategories = computed(() => props.categories.filter(item => item.kind === props.kind))
const roots = computed(() => kindCategories.value.filter(item => !item.parent_id).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')))
// Only a real root has children. Without the guard the roots would show up
// here, because their parent id is zero as well.
const children = computed(() => rootID.value > 0 ? kindCategories.value.filter(item => item.parent_id === rootID.value).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')) : [])
const legacy = computed(() => props.modelValue && !kindCategories.value.some(item => item.name === props.modelValue) ? props.modelValue : '')
const childPlaceholder = computed(() => {
  if (!rootID.value || rootID.value === -1) return '请先选择一级分类'
  return children.value.length ? '请选择二级分类' : '该分类没有二级分类'
})
// The path of the current choice, so the user sees which level is active.
const hint = computed(() => {
  if (rootID.value === -1) return '历史分类，已不在当前分类列表中'
  const root = roots.value.find(item => item.id === rootID.value)
  if (!root) return ''
  const child = children.value.find(item => item.id === childID.value)
  if (child) return `已选：${root.name} / ${child.name}`
  return children.value.length ? `已选一级分类「${root.name}」，请选择二级分类` : `已选：${root.name}`
})

function sync() {
  const selected = kindCategories.value.find(item => item.name === props.modelValue)
  if (!selected) {
    if (!props.modelValue && roots.value.some(item => item.id === rootID.value)) return
    rootID.value = legacy.value ? -1 : 0; childID.value = 0; return
  }
  rootID.value = selected.parent_id || selected.id
  childID.value = selected.parent_id ? selected.id : 0
}
// Picking a root fills the second select with its own categories at once.
function selectRoot() {
  if (rootID.value === -1) { emit('update:modelValue', legacy.value); return }
  childID.value = 0
  const root = roots.value.find(item => item.id === rootID.value)
  emit('update:modelValue', root && !children.value.length ? root.name : '')
}
function selectChild() { emit('update:modelValue', children.value.find(item => item.id === childID.value)?.name || '') }
// A quick pick fills the category right away, the cascade follows on its own.
function pick(name: string) { emit('update:modelValue', name) }
// The categories the user records most often, so a bill can be filed with one
// click.
async function loadRecent() {
  try {
    const response = await billingApi.personalRecentCategories(shareStore.currentOwnerId, props.kind)
    const data = response.data?.data
    recent.value = (data?.items || []) as PersonalRecentCategory[]
  } catch {
    recent.value = []
  }
}
watch(() => [props.modelValue, props.kind, props.categories] as const, sync, { immediate: true, deep: true })
watch(() => [props.kind, shareStore.currentOwnerId], loadRecent, { immediate: true })
</script>
