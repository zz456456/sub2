<template>
  <div ref="root" class="relative" @keydown.esc="open = false">
    <button
      v-if="authStore.isAdmin"
      type="button"
      class="flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs transition-colors"
      :class="hasUpdate
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
        : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-400'"
      :title="t('version.checkOwnUpdates')"
      :aria-expanded="open"
      aria-haspopup="dialog"
      @click="open = !open"
    >
      <span>{{ currentVersion ? `v${currentVersion}` : '--' }}</span>
      <span v-if="hasUpdate" class="h-2 w-2 rounded-full bg-amber-500"></span>
    </button>
    <span v-else-if="version" class="text-xs text-gray-500 dark:text-dark-400">v{{ version }}</span>

    <div
      v-if="authStore.isAdmin && open"
      role="dialog"
      :aria-label="t('version.checkOwnUpdates')"
      class="absolute left-0 z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] space-y-3 rounded-xl border border-gray-200 bg-white p-4 text-sm shadow-lg dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="flex items-center justify-between gap-2">
        <span class="font-medium text-gray-900 dark:text-white">{{ t('version.currentVersion') }}: v{{ currentVersion }}</span>
        <button
          type="button"
          :disabled="loading"
          class="text-primary-600 disabled:opacity-50 dark:text-primary-400"
          @click="refresh(true)"
        >{{ loading ? t('common.loading') : t('version.refresh') }}</button>
      </div>

      <p v-if="errorMessage" role="status" class="break-words text-amber-700 dark:text-amber-400">
        {{ errorMessage }}
      </p>
      <template v-else-if="info">
        <p :class="hasUpdate ? 'text-amber-700 dark:text-amber-400' : 'text-gray-600 dark:text-dark-300'">
          {{ hasUpdate ? t('version.updateAvailable') : t('version.upToDate') }}
        </p>
        <p class="text-gray-600 dark:text-dark-300">{{ t('version.latestVersion') }}: v{{ info.latest_version }}</p>
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('version.dockerUpdateHint') }}</p>
        <code class="block break-all rounded bg-gray-100 p-2 text-xs dark:bg-dark-900">image: {{ dockerImage }}:latest</code>
        <pre class="overflow-x-auto whitespace-pre-wrap break-words rounded bg-gray-100 p-2 text-xs dark:bg-dark-900">{{ updateCommands }}</pre>
        <button type="button" class="text-primary-600 dark:text-primary-400" @click="copyToClipboard(updateCommands)">
          {{ copied ? t('version.copied') : t('version.copyCommand') }}
        </button>
      </template>

      <a
        :href="`https://github.com/${repository}/releases`"
        target="_blank"
        rel="noopener noreferrer"
        class="block break-all text-xs text-primary-600 dark:text-primary-400"
      >{{ t('version.viewRelease') }} · {{ repository }}</a>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { onClickOutside } from '@vueuse/core'
import { useAuthStore } from '@/stores/auth'
import { useClipboard } from '@/composables/useClipboard'
import { checkUpdates, type VersionInfo } from '@/api/admin/system'

const props = defineProps<{ version?: string }>()
const { t } = useI18n()
const authStore = useAuthStore()
const { copied, copyToClipboard } = useClipboard()
const root = ref<HTMLElement | null>(null)
const open = ref(false)
const loading = ref(false)
const info = ref<VersionInfo | null>(null)
const errorMessage = ref('')
const currentVersion = computed(() => info.value?.current_version || props.version || '')
const hasUpdate = computed(() => !errorMessage.value && !!info.value?.has_update)
const repository = computed(() => info.value?.update_repository || 'zz456456/sub2')
const dockerImage = computed(() => info.value?.docker_image || 'ghcr.io/zz456456/sub2')
const updateCommands = 'docker compose pull sub2api\ndocker compose up -d --no-deps sub2api'

onClickOutside(root, () => { open.value = false })

async function refresh(force = false) {
  if (!authStore.isAdmin || loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    info.value = await checkUpdates(force)
    if (info.value.warning) {
      errorMessage.value = info.value.warning.includes('GitHub API returned 404')
        ? t('version.noCustomRelease')
        : info.value.warning
    }
  } catch {
    errorMessage.value = t('version.checkFailed')
  } finally {
    loading.value = false
  }
}

watch(() => authStore.isAdmin, (isAdmin) => {
  if (isAdmin) void refresh()
  else {
    open.value = false
    info.value = null
  }
}, { immediate: true })
</script>
