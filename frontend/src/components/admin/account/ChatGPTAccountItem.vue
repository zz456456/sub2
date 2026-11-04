<template>
  <component
    :is="layout === 'list' ? 'tr' : 'article'"
    :class="[
      layout === 'list'
        ? 'border-b border-gray-100 dark:border-dark-700'
        : 'grid min-w-0 grid-cols-2 gap-4 rounded-xl border p-4 transition-colors',
      layout === 'cards' && selected ? 'ring-2 ring-primary-500' : ''
    ]"
    :data-account-id="account.id"
    :aria-label="account.name"
    class="border-gray-200 dark:border-dark-700"
  >
    <component :is="cellTag" :class="cellClass">
      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          :checked="selected"
          :aria-label="p('selectAccount', { name: account.name })"
          class="h-4 w-4 rounded border-gray-300 text-primary-600"
          :disabled="busy"
          @change="$emit('select', account.id)"
        />
        <span class="font-mono text-xs text-gray-400">#{{ account.id }}</span>
      </label>
    </component>
    <component :is="cellTag" :class="cellClass">
      <div v-if="layout === 'cards'" class="mb-1 text-xs text-gray-400">
        AT / RT
      </div>
      <ChatGPTCredentialStatus
        :state="account.credential_state"
        :copyable="!busy"
        @copy="$emit('copy', account, $event)"
      />
    </component>
    <component :is="cellTag" :class="cellClass">
      <div class="flex flex-wrap items-center gap-1.5 text-xs">
        <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">{{
          account.credentials?.source_type === 'codex' ? 'Codex' : 'Web'
        }}</span>
        <span class="font-medium text-gray-700 dark:text-gray-200">{{
          snapshot?.plan || account.credentials?.plan_type || p('unknown')
        }}</span>
      </div>
    </component>
    <component :is="cellTag" :class="cellClass">
      <span
        class="inline-block rounded-full px-2.5 py-1 text-xs font-medium"
        :class="statusClass"
        >{{
          p(`status.${account.chatgpt_pool_state?.status || 'pending'}`)
        }}</span
      >
    </component>
    <component
      :is="cellTag"
      :class="[
        cellClass,
        layout === 'cards' ? 'col-span-2 min-w-0' : 'max-w-[16rem]'
      ]"
    >
      <div
        class="truncate font-medium text-gray-900 dark:text-white"
        :title="account.name"
      >
        {{ account.name }}
      </div>
      <div class="truncate text-xs text-gray-500" :title="email">
        {{ email || '—' }}
      </div>
      <div v-if="groups.length" class="mt-1 flex flex-wrap gap-1">
        <span
          v-for="group in groups"
          :key="group.id"
          class="rounded bg-primary-50 px-1.5 py-0.5 text-[11px] text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
          >{{ group.name }}</span
        >
      </div>
      <div v-if="account.proxy" class="mt-1 truncate text-xs text-gray-400">
        {{ p('proxy') }}: {{ account.proxy.name }}
      </div>
      <div
        v-if="account.notes"
        class="mt-1 truncate text-xs text-gray-400"
        :title="account.notes"
      >
        {{ account.notes }}
      </div>
    </component>
    <component :is="cellTag" :class="cellClass">
      <div class="text-xs text-gray-400">{{ p('imageQuota') }}</div>
      <span
        class="font-mono text-lg font-semibold"
        :class="
          account.chatgpt_pool_state?.quota === 'exhausted'
            ? 'text-amber-600'
            : 'text-gray-800 dark:text-gray-100'
        "
        >{{ quotaLabel }}</span
      >
      <div
        v-if="snapshot?.synced_at"
        class="text-[11px] text-gray-400"
        :title="formatDateTime(snapshot.synced_at)"
      >
        {{ p('lastSync') }} {{ formatRelativeTime(snapshot.synced_at) }}
      </div>
      <div
        v-if="account.extra?.chatgpt_last_sync_error"
        class="mt-1 max-w-[14rem] truncate text-xs text-red-500"
        :title="String(account.extra.chatgpt_last_sync_error)"
      >
        {{ account.extra.chatgpt_last_sync_error }}
      </div>
    </component>
    <component :is="cellTag" :class="cellClass">
      <div class="text-xs text-gray-400">{{ p('resetAt') }}</div>
      <span
        class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300"
        >{{
          snapshot?.quota_reset_at
            ? formatDateTime(snapshot.quota_reset_at)
            : '—'
        }}</span
      >
      <div class="mt-1 text-[11px] text-gray-400">
        {{ p('createdAt') }} {{ formatDateTime(account.created_at) }}
      </div>
    </component>
    <component
      :is="cellTag"
      :class="[
        cellClass,
        layout === 'cards'
          ? 'col-span-2 border-t border-gray-100 pt-3 dark:border-dark-700'
          : ''
      ]"
    >
      <div class="flex flex-wrap items-center gap-1.5">
        <button
          class="btn btn-secondary btn-sm"
          :disabled="busy"
          @click="$emit('edit', account)"
        >
          {{ p('edit') }}
        </button>
        <button
          class="btn btn-secondary btn-sm"
          :disabled="busy"
          @click="$emit('sync', account)"
        >
          {{ p('sync') }}
        </button>
        <button
          class="btn btn-secondary btn-sm"
          :disabled="busy || !account.credential_state?.can_refresh"
          @click="$emit('refresh', account)"
        >
          {{ p('refreshAT') }}
        </button>
        <button
          class="btn btn-secondary btn-sm"
          :disabled="busy"
          @click="$emit('toggle', account)"
        >
          {{
            account.status === 'active' && account.schedulable
              ? p('disable')
              : p('enable')
          }}
        </button>
        <button
          class="rounded px-2 py-1 text-xs text-red-600 hover:bg-red-50 disabled:opacity-50 dark:hover:bg-red-900/20"
          :disabled="busy"
          @click="$emit('delete', account)"
        >
          {{ p('delete') }}
        </button>
      </div>
    </component>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminGroup } from '@/types'
import {
  chatgptSnapshot,
  type ChatGPTAccount
} from '@/api/admin/chatgptAccounts'
import ChatGPTCredentialStatus from './ChatGPTCredentialStatus.vue'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const props = defineProps<{
  account: ChatGPTAccount
  layout: 'list' | 'cards'
  selected: boolean
  busy: boolean
  groups: AdminGroup[]
}>()
defineEmits<{
  select: [id: number]
  edit: [account: ChatGPTAccount]
  sync: [account: ChatGPTAccount]
  refresh: [account: ChatGPTAccount]
  toggle: [account: ChatGPTAccount]
  delete: [account: ChatGPTAccount]
  copy: [account: ChatGPTAccount, kind: 'access_token' | 'refresh_token']
}>()
const { t } = useI18n()
const p = (key: string, values: Record<string, string | number> = {}) =>
  t(`admin.chatgptAccounts.${key}`, values)
const cellTag = computed(() => (props.layout === 'list' ? 'td' : 'div'))
const cellClass = computed(() =>
  props.layout === 'list' ? 'px-4 py-4 align-top' : 'min-w-0'
)
const snapshot = computed(() => chatgptSnapshot(props.account))
const email = computed(() =>
  String(snapshot.value?.email || props.account.credentials?.email || '')
)
const quotaLabel = computed(() => {
  const state = props.account.chatgpt_pool_state?.quota
  if (state === 'unlimited') return '∞'
  if (!state || state === 'unknown') return p('unknown')
  return String(snapshot.value?.quota_remaining ?? p('unknown'))
})
const statusClass = computed(() => {
  switch (props.account.chatgpt_pool_state?.status) {
    case 'normal':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    case 'abnormal':
      return 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
    case 'limited':
    case 'recoverable':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300'
  }
})
</script>
