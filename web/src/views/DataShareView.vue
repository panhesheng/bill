<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="数据共享" description="把你的账单数据指定分享给其他账号，他们就能在自己的界面上查看你的账户、账单、统计分析与首页概览。">
      <template #actions>
        <button type="button" class="btn-brand min-h-11" :disabled="saving || !keyword.trim()" @click="addShare">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v14m-7-7h14" /></svg>
          分享给用户
        </button>
      </template>
    </PageHeader>

    <section class="surface rounded-[1.25rem] p-4 sm:p-6">
      <h2 class="font-display text-lg font-bold">添加分享对象</h2>
      <p class="mt-1 text-sm leading-6 text-muted-foreground">输入对方的用户名，确认后对方即可查看你的数据。对方只能查看，无法修改你的账单与账户。</p>
      <div class="mt-4 flex flex-col gap-2 sm:flex-row">
        <label class="relative flex-1">
          <span class="sr-only">用户名</span>
          <input v-model="keyword" class="input-field" placeholder="例如 alice" autocomplete="off" @input="searchUsers" @keydown.enter.prevent="addShare" />
          <ul v-if="suggestions.length" class="absolute z-20 mt-1 w-full overflow-hidden rounded-xl border border-border bg-surface shadow-xl">
            <li v-for="item in suggestions" :key="item.id">
              <button type="button" class="flex w-full items-center justify-between px-3 py-2.5 text-left text-sm hover:bg-muted" @click="pickUser(item)">
                <span class="font-semibold">{{ item.username }}</span><span class="text-xs text-muted-foreground">选择</span>
              </button>
            </li>
          </ul>
        </label>
        <button type="button" class="btn-brand min-h-11 sm:w-auto" :disabled="saving || !keyword.trim()" @click="addShare">分享</button>
      </div>
      <p v-if="formError" class="mt-2 text-sm font-semibold text-rose-600" role="alert">{{ formError }}</p>
      <p v-else-if="successMessage" class="mt-2 text-sm font-semibold text-emerald-600" role="status">{{ successMessage }}</p>
    </section>

    <section aria-labelledby="viewers-title" class="mt-6">
      <div class="mb-3 flex items-center justify-between gap-3">
        <div><h2 id="viewers-title" class="font-display text-xl font-bold">我分享给的用户</h2><p class="mt-1 text-xs text-muted-foreground">这些账号可以查看你的全部个人账单数据。</p></div>
        <span class="badge bg-muted text-muted-foreground">{{ shareStore.viewers.length }} 人</span>
      </div>
      <div v-if="shareStore.loading" class="surface h-24 animate-pulse rounded-2xl bg-muted/60"></div>
      <div v-else-if="!shareStore.viewers.length" class="empty-state surface min-h-48 rounded-[1.25rem] border-dashed">
        <svg class="h-10 w-10 text-muted-foreground/60" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M17 20h5v-2a3 3 0 0 0-5.4-1.8M17 20H7m10 0v-2c0-.7-.1-1.3-.4-1.8M7 20H2v-2a3 3 0 0 1 5.4-1.8M7 20v-2c0-.7.1-1.3.4-1.8m0 0a5 5 0 0 1 9.2 0M15 7a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" /></svg>
        <strong class="mt-3 text-foreground">还没有分享给任何人</strong><span class="mt-1">在上面输入用户名即可授权</span>
      </div>
      <ul v-else class="grid gap-3 sm:grid-cols-2">
        <li v-for="viewer in shareStore.viewers" :key="viewer.id" class="surface flex items-center justify-between gap-3 rounded-2xl p-4">
          <div class="min-w-0"><p class="truncate font-semibold">{{ viewer.username }}</p><p class="mt-0.5 text-xs text-muted-foreground">授权于 {{ viewer.created_at }}</p></div>
          <button type="button" class="min-h-11 shrink-0 px-3 text-sm font-semibold text-rose-600" @click="removeShare(viewer)">取消共享</button>
        </li>
      </ul>
    </section>

    <section aria-labelledby="owners-title" class="mt-6">
      <div class="mb-3 flex items-center justify-between gap-3">
        <div><h2 id="owners-title" class="font-display text-xl font-bold">共享给我的数据</h2><p class="mt-1 text-xs text-muted-foreground">这些用户把数据分享给了你，你可以随时切换数据源查看。</p></div>
        <span class="badge bg-muted text-muted-foreground">{{ shareStore.owners.length }} 人</span>
      </div>
      <div v-if="!shareStore.owners.length" class="empty-state surface min-h-48 rounded-[1.25rem] border-dashed">
        <svg class="h-10 w-10 text-muted-foreground/60" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M12 4.5v15m7.5-7.5h-15" /></svg>
        <strong class="mt-3 text-foreground">还没有人分享数据给你</strong><span class="mt-1">对方授权后会显示在这里</span>
      </div>
      <ul v-else class="grid gap-3 sm:grid-cols-2">
        <li v-for="owner in shareStore.owners" :key="owner.user_id" class="surface flex items-center justify-between gap-3 rounded-2xl p-4">
          <div class="min-w-0"><p class="truncate font-semibold">{{ owner.username }}</p><p class="mt-0.5 text-xs text-muted-foreground">只读查看</p></div>
          <button type="button" class="min-h-11 shrink-0 px-3 text-sm font-semibold text-brand-700 dark:text-brand-300" @click="viewOwner(owner)">查看数据</button>
        </li>
      </ul>
    </section>

    <Toast :message="toast.message" :type="toast.type" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { billingApi, type DataShareOwner, type DataShareViewer } from '../api/billing'
import { apiMessage } from '../utils/billing'
import { useDataShareStore } from '../stores/dataShare'
import PageHeader from '../components/PageHeader.vue'
import Toast from '../components/Toast.vue'

const router = useRouter()
const shareStore = useDataShareStore()
const keyword = ref('')
const saving = ref(false)
const formError = ref('')
const successMessage = ref('')
const suggestions = ref<{ id: number; username: string }[]>([])
const toast = reactive<{ message: string; type: 'success' | 'error' }>({ message: '', type: 'success' })

function notify(message: string, type: 'success' | 'error' = 'success') {
  toast.message = ''
  setTimeout(() => {
    toast.message = message
    toast.type = type
  })
}

let searchTimer = 0
function searchUsers() {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(async () => {
    const query = keyword.value.trim()
    if (!query) {
      suggestions.value = []
      return
    }
    try {
      const res = await billingApi.users(query)
      const data = res.data?.data
      const list = Array.isArray(data) ? data : []
      suggestions.value = list
        .filter((item: any) => !shareStore.viewers.some((viewer: DataShareViewer) => viewer.user_id === item.id))
        .slice(0, 8)
    } catch {
      suggestions.value = []
    }
  }, 250)
}

function pickUser(item: { id: number; username: string }) {
  keyword.value = item.username
  suggestions.value = []
}

async function addShare() {
  const name = keyword.value.trim()
  if (!name || saving.value) return
  saving.value = true
  formError.value = ''
  successMessage.value = ''
  try {
    const res = await billingApi.personalShareCreate({ username: name })
    if (res.data?.code === 0) {
      successMessage.value = `已把数据分享给 ${name}`
      keyword.value = ''
      suggestions.value = []
      await shareStore.refresh()
    } else {
      formError.value = res.data?.message || '共享失败'
    }
  } catch (error) {
    formError.value = apiMessage(error, '共享失败')
  } finally {
    saving.value = false
  }
}

async function removeShare(viewer: DataShareViewer) {
  try {
    const res = await billingApi.personalShareDelete(viewer.id)
    if (res.data?.code === 0) {
      notify(`已取消与 ${viewer.username} 的共享`)
      await shareStore.refresh()
    }
  } catch (error) {
    notify(apiMessage(error, '取消共享失败'), 'error')
  }
}

function viewOwner(owner: DataShareOwner) {
  shareStore.selectOwner(owner.user_id)
  router.push('/admin/personal')
}

onMounted(() => shareStore.refresh())
</script>
