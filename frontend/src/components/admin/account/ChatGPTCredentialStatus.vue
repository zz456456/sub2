<template>
  <div class="flex flex-wrap gap-1.5">
    <button
      v-for="kind in kinds"
      :key="kind.key"
      type="button"
      class="rounded-md border px-2 py-1 text-xs font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 disabled:cursor-default"
      :class="tone(kind.state)"
      :title="tooltip(kind.key)"
      :disabled="
        !copyable || kind.state === 'missing' || kind.state === 'inherited'
      "
      @click="$emit('copy', kind.key)"
    >
      {{ kind.label }} ·
      {{ t(`admin.chatgptAccounts.credentials.${kind.state}`) }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CredentialState } from '@/api/admin/chatgptAccounts'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ state?: CredentialState; copyable?: boolean }>()
defineEmits<{ copy: [kind: 'access_token' | 'refresh_token'] }>()
const { t } = useI18n()
const kinds = computed(() => [
  {
    key: 'access_token' as const,
    label: 'AT',
    state: props.state?.access ?? 'unknown'
  },
  {
    key: 'refresh_token' as const,
    label: 'RT',
    state: props.state?.refresh ?? 'unknown'
  }
])
function tone(state: string) {
  if (state === 'valid' || state === 'present')
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-300'
  if (state === 'invalid' || state === 'expired')
    return 'border-red-200 bg-red-50 text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300'
  if (state === 'expiring')
    return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300'
  return 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300'
}
function tooltip(key: string) {
  const state = props.state
  const lines: string[] = []
  if (key === 'access_token') {
    if (state?.issued_at)
      lines.push(
        `${t('admin.chatgptAccounts.issuedAt')}: ${formatDateTime(new Date(state.issued_at * 1000).toISOString())}`
      )
    if (state?.expires_at)
      lines.push(
        `${t('admin.chatgptAccounts.expiresAt')}: ${formatDateTime(new Date(state.expires_at * 1000).toISOString())}`
      )
  }
  if (props.copyable) lines.push(t('admin.chatgptAccounts.copyCredential'))
  return lines.join('\n')
}
</script>
