import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { billingApi, type DataShareOwner, type DataShareViewer } from '../api/billing'

const STORAGE_KEY = 'bill.dataShare.ownerId'

// The data source of the personal pages. It is either the data of the signed
// in user (ownerId 0) or the data of a user that shared it with them.
export const useDataShareStore = defineStore('dataShare', () => {
  const viewers = ref<DataShareViewer[]>([])
  const owners = ref<DataShareOwner[]>([])
  const currentOwnerId = ref<number>(Number(localStorage.getItem(STORAGE_KEY)) || 0)
  const loading = ref(false)

  const isSharedView = computed(() => currentOwnerId.value > 0)
  const currentOwnerName = computed(() => owners.value.find(item => item.user_id === currentOwnerId.value)?.username || '')
  // The parameter every read request carries while another data source is
  // selected.
  const ownerParam = computed(() => (currentOwnerId.value > 0 ? { owner_id: currentOwnerId.value } : {}))

  async function refresh() {
    loading.value = true
    try {
      const res = await billingApi.personalShares()
      if (res.data?.code === 0) {
        viewers.value = res.data.data?.viewers || []
        owners.value = res.data.data?.owners || []
        // Drop a selection whose owner revoked the share.
        if (currentOwnerId.value > 0 && !owners.value.some(item => item.user_id === currentOwnerId.value)) {
          selectOwner(0)
        }
      }
    } catch {
    } finally {
      loading.value = false
    }
  }

  function selectOwner(userId: number) {
    currentOwnerId.value = userId || 0
    if (currentOwnerId.value > 0) localStorage.setItem(STORAGE_KEY, String(currentOwnerId.value))
    else localStorage.removeItem(STORAGE_KEY)
  }

  return { viewers, owners, currentOwnerId, isSharedView, currentOwnerName, ownerParam, loading, refresh, selectOwner }
})
