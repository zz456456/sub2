<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="mb-4 flex flex-wrap items-center justify-end gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <button
              class="btn btn-secondary"
              :disabled="!!busy"
              @click="showGroups = true"
            >
              {{ p('manageGroups') }}
            </button>
            <button
              class="btn btn-secondary"
              :disabled="!!busy"
              @click="openImport"
            >
              {{ p('import') }}
            </button>
            <button
              class="btn btn-primary"
              :disabled="!!busy"
              @click="openEditor(null)"
            >
              <Icon name="plus" size="sm" />{{ p('add') }}
            </button>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <SearchInput
            v-model="filters.search"
            :placeholder="p('search')"
            class="min-w-48 flex-1 sm:max-w-sm"
          />
          <Select
            v-model="filters.status"
            :options="statusOptions"
            class="w-36"
            :aria-label="p('statusFilter')"
          />
          <Select
            v-model="filters.source"
            :options="sourceOptions"
            class="w-32"
            :aria-label="p('source')"
          />
          <Select
            v-model="filters.group"
            :options="groupOptions"
            class="w-40"
            :aria-label="p('group')"
          />
          <div class="ml-auto flex items-center gap-2">
            <label class="flex items-center gap-1.5 text-xs text-gray-500"
              ><input v-model="autoRefresh" type="checkbox" />{{
                p('autoRefresh')
              }}</label
            >
            <button
              class="btn btn-secondary px-2"
              :disabled="loading"
              :aria-label="p('reload')"
              @click="load"
            >
              <Icon
                name="refresh"
                size="sm"
                :class="loading ? 'animate-spin' : ''"
              />
            </button>
            <div
              class="flex overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"
              role="group"
              :aria-label="p('viewMode')"
            >
              <button
                v-for="mode in viewModes"
                :key="mode"
                class="px-3 py-2 text-xs font-medium"
                :class="
                  viewMode === mode
                    ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                    : 'text-gray-500'
                "
                :aria-pressed="viewMode === mode"
                @click="setViewMode(mode)"
              >
                {{ p(mode) }}
              </button>
            </div>
          </div>
        </div>
        <div
          class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500"
        >
          <span>{{ p('total', { count: total }) }}</span>
          <span
            v-for="state in [
              'normal',
              'limited',
              'abnormal',
              'recoverable',
              'disabled'
            ]"
            :key="state"
            >{{ p(`status.${state}`) }}
            <strong class="text-gray-700 dark:text-gray-200">{{
              counts[state] || 0
            }}</strong></span
          >
          <span v-if="lastLoaded" class="ml-auto"
            >{{ p('updated') }} {{ formatDateTime(lastLoaded) }}</span
          >
        </div>
        <div
          class="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-primary-50 px-3 py-2 dark:bg-primary-900/20"
        >
          <label
            class="flex items-center gap-2 text-xs font-medium text-primary-800 dark:text-primary-200"
            ><input
              type="checkbox"
              :checked="allPageSelected"
              :indeterminate="somePageSelected"
              :disabled="!!busy || !accounts.length"
              @change="selectPage(($event.target as HTMLInputElement).checked)"
            />{{ p('selectPage') }}</label
          >
          <span class="text-xs text-primary-700 dark:text-primary-300">{{
            p('selected', { count: selected.size })
          }}</span>
          <button
            class="text-xs text-primary-700 disabled:opacity-50 dark:text-primary-300"
            :disabled="!!busy || selecting || !total"
            @click="selectAll"
          >
            {{ selecting ? p('selecting') : p('selectAll') }}
          </button>
          <button
            v-if="selected.size"
            class="text-xs text-primary-700 dark:text-primary-300"
            :disabled="!!busy"
            @click="selected = new Set()"
          >
            {{ p('clearSelection') }}
          </button>
          <Select
            v-model="bulkAction"
            :options="actionOptions"
            class="ml-auto w-44"
            :disabled="!!busy || !selected.size"
            :aria-label="p('bulkActions')"
            @change="onBulkAction"
          />
          <Select
            v-model="exportFormat"
            :options="exportOptions"
            class="w-44"
            :disabled="!!busy || !total"
            :aria-label="p('export')"
            @change="onExport"
          />
          <button
            v-if="operationItems.length || busy"
            class="btn btn-secondary btn-sm"
            @click="showOperation = true"
          >
            {{ p('results')
            }}<span v-if="busy"> {{ processed }}/{{ operationTotal }}</span>
          </button>
        </div>
        <div
          v-if="loadError"
          role="alert"
          class="mt-2 flex items-center justify-between rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300"
        >
          <span>{{ loadError }}</span
          ><button class="underline" @click="load">{{ p('retry') }}</button>
        </div>
      </template>
      <template #table>
        <div
          class="pool-scroll min-h-0 flex-1 overflow-auto"
          :aria-busy="loading"
        >
          <div
            v-if="loading && !accounts.length"
            class="flex min-h-64 items-center justify-center text-gray-400"
          >
            <Icon name="refresh" size="md" class="mr-2 animate-spin" />{{
              p('loading')
            }}
          </div>
          <div
            v-else-if="!accounts.length"
            class="flex min-h-64 flex-col items-center justify-center gap-3 p-6 text-center text-gray-500"
          >
            <Icon name="users" size="xl" />
            <p>{{ p('empty') }}</p>
            <button class="btn btn-primary" @click="openImport">
              {{ p('import') }}
            </button>
          </div>
          <component
            :is="viewMode === 'list' ? 'table' : 'div'"
            v-else
            :class="
              viewMode === 'list'
                ? 'w-full min-w-[1100px] text-left'
                : 'grid grid-cols-1 gap-4 p-4 md:grid-cols-2 2xl:grid-cols-3'
            "
          >
            <thead
              v-if="viewMode === 'list'"
              class="sticky top-0 z-10 bg-gray-50 text-xs text-gray-500 dark:bg-dark-800"
            >
              <tr>
                <th
                  v-for="column in [
                    'id',
                    'credentialsTitle',
                    'sourcePlan',
                    'statusFilter',
                    'accountInfo',
                    'imageQuota',
                    'resetAt',
                    'actions'
                  ]"
                  :key="column"
                  class="px-4 py-3 font-medium"
                >
                  {{ p(column) }}
                </th>
              </tr>
            </thead>
            <component
              :is="viewMode === 'list' ? 'tbody' : 'div'"
              :class="viewMode === 'cards' ? 'contents' : ''"
            >
              <ChatGPTAccountItem
                v-for="account in accounts"
                :key="account.id"
                :account="account"
                :layout="viewMode"
                :selected="selected.has(account.id)"
                :busy="!!busy"
                :groups="accountGroups(account)"
                @select="toggleSelect"
                @edit="openEditor"
                @sync="runOperation('sync', [$event.id])"
                @refresh="runOperation('refresh', [$event.id])"
                @toggle="
                  runOperation(
                    $event.status === 'active' && $event.schedulable
                      ? 'disable'
                      : 'enable',
                    [$event.id]
                  )
                "
                @delete="requestDelete([$event.id])"
                @copy="copyCredential"
              />
            </component>
          </component>
        </div>
      </template>
      <template #pagination
        ><Pagination
          v-if="total > 0"
          :page="page"
          :total="total"
          :page-size="pageSize"
          @update:page="changePage"
          @update:pageSize="changePageSize"
      /></template>
    </TablePageLayout>

    <BaseDialog
      :show="showImport"
      :title="p('importTitle')"
      width="wide"
      :close-on-escape="!importing"
      :show-close-button="!importing"
      @close="closeImport"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-500">{{ p('importHint') }}</p>
        <div class="grid gap-3 sm:grid-cols-2">
          <label
            ><span class="input-label">{{ p('source') }}</span
            ><Select
              v-model="importSource"
              :options="sourceOptions.filter((option) => option.value)"
              :disabled="importing" /></label
          ><label
            ><span class="input-label">{{ p('group') }}</span
            ><Select
              v-model="importGroup"
              :options="bindingGroupOptions"
              :disabled="importing"
          /></label>
        </div>
        <label class="block"
          ><span class="input-label">{{ p('proxy') }}</span
          ><Select
            v-model="importProxy"
            :options="proxyOptions"
            :disabled="importing"
        /></label>
        <textarea
          v-model="importContent"
          class="input font-mono text-xs"
          rows="7"
          :placeholder="p('importPlaceholder')"
          autocomplete="off"
          spellcheck="false"
          :disabled="importing"
        />
        <label
          class="flex cursor-pointer items-center gap-2 rounded-lg border border-dashed border-gray-300 p-4 text-sm dark:border-dark-600"
          ><Icon name="upload" size="sm" /><span
            >{{ p('chooseFiles') }}
            {{ importFiles.map((file) => file.name).join(', ') }}</span
          ><input
            type="file"
            accept=".json,.txt,application/json,text/plain"
            multiple
            class="sr-only"
            :disabled="importing"
            @change="chooseFiles"
        /></label>
        <label class="flex items-center gap-2 text-sm"
          ><input
            v-model="updateExisting"
            type="checkbox"
            :disabled="importing"
          />{{ p('updateExisting') }}</label
        >
        <details
          class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600"
        >
          <summary class="cursor-pointer font-medium">
            {{ p('remoteImport') }}
          </summary>
          <div class="mt-3 space-y-3">
            <p class="text-xs text-gray-500">{{ p('remoteHint') }}</p>
            <Select
              v-model="remoteKind"
              :options="[
                { value: 'cpa', label: 'CPA' },
                { value: 'sub2api', label: 'Sub2API' }
              ]"
            /><input
              v-model.trim="remoteURL"
              class="input"
              placeholder="https://example.com"
              :disabled="importing"
            /><input
              v-model="remoteKey"
              type="password"
              class="input"
              :placeholder="p('remoteKey')"
              autocomplete="off"
              :disabled="importing"
            /><button
              class="btn btn-secondary btn-sm"
              :disabled="importing || !remoteURL || !remoteKey"
              @click="importRemote"
            >
              {{ importing ? p('importing') : p('remoteImport') }}
            </button>
          </div>
        </details>
        <details
          class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600"
        >
          <summary class="cursor-pointer font-medium">
            {{ p('oauthLogin') }}
          </summary>
          <div class="mt-3 space-y-3">
            <p class="text-xs text-gray-500">{{ p('oauthHint') }}</p>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="oauthLoading || importing"
              @click="generateOAuth"
            >
              {{ p('generateOAuth') }}</button
            ><a
              v-if="oauthUrl"
              :href="oauthUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="ml-3 break-all text-xs text-primary-600 underline"
              >{{ p('openOAuth') }}</a
            ><input
              v-if="oauthUrl"
              v-model="oauthCallback"
              class="input"
              :placeholder="p('oauthCallback')"
              autocomplete="off"
            /><button
              v-if="oauthUrl"
              class="btn btn-secondary btn-sm"
              :disabled="oauthLoading || importing || !oauthCallback"
              @click="exchangeOAuth"
            >
              {{ p('exchangeOAuth') }}
            </button>
          </div>
        </details>
        <div
          v-if="importResult"
          class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-800"
          role="status"
        >
          <p>
            {{
              p('importResult', {
                created: importResult.created,
                updated: importResult.updated,
                skipped: importResult.skipped,
                failed: importResult.failed
              })
            }}
          </p>
          <ul
            v-if="importResult.errors?.length || importResult.warnings?.length"
            class="mt-2 max-h-48 space-y-1 overflow-auto text-xs"
          >
            <li
              v-for="(item, index) in [
                ...(importResult.errors || []),
                ...(importResult.warnings || [])
              ]"
              :key="index"
            >
              #{{ item.index }} {{ item.name }} — {{ item.message }}
            </li>
          </ul>
        </div>
      </div>
      <template #footer
        ><div class="flex justify-end gap-2">
          <button
            class="btn btn-secondary"
            :disabled="importing"
            @click="closeImport"
          >
            {{ p('close') }}</button
          ><button
            class="btn btn-primary"
            :disabled="
              importing || (!importContent.trim() && !importFiles.length)
            "
            @click="importAccounts"
          >
            {{ importing ? p('importing') : p('import') }}
          </button>
        </div></template
      >
    </BaseDialog>

    <BaseDialog
      :show="showEditor"
      :title="editing ? p('edit') : p('add')"
      :show-close-button="!saving"
      :close-on-escape="!saving"
      @close="closeEditor"
    >
      <form
        id="chatgpt-account-form"
        class="space-y-4"
        @submit.prevent="saveAccount"
      >
        <label class="block"
          ><span class="input-label">{{ p('accountName') }}</span
          ><input
            v-model.trim="draft.name"
            class="input"
            required
            maxlength="100"
        /></label>
        <div class="grid gap-3 sm:grid-cols-2">
          <label
            ><span class="input-label">{{ p('email') }}</span
            ><input v-model.trim="draft.email" class="input" /></label
          ><label
            ><span class="input-label">{{ p('plan') }}</span
            ><input
              v-model.trim="draft.plan"
              class="input"
              placeholder="Free / Plus / Pro / Team"
          /></label>
        </div>
        <label class="block"
          ><span class="input-label">Access Token</span
          ><textarea
            v-model.trim="draft.access"
            class="input font-mono text-xs"
            rows="3"
            :required="!editing"
            :placeholder="editing ? p('keepCredential') : ''"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <label class="block"
          ><span class="input-label">Refresh Token</span
          ><textarea
            v-model.trim="draft.refresh"
            class="input font-mono text-xs"
            rows="2"
            :placeholder="editing ? p('keepCredential') : p('optional')"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <div class="grid gap-3 sm:grid-cols-2">
          <label
            ><span class="input-label">{{ p('source') }}</span
            ><Select
              v-model="draft.source"
              :options="
                sourceOptions.filter((option) => option.value)
              " /></label
          ><label
            ><span class="input-label">{{ p('proxy') }}</span
            ><Select v-model="draft.proxy" :options="proxyOptions"
          /></label>
        </div>
        <fieldset v-if="groups.length">
          <legend class="input-label">{{ p('group') }}</legend>
          <div class="flex flex-wrap gap-3">
            <label
              v-for="group in groups"
              :key="group.id"
              class="flex items-center gap-1.5 text-sm"
              ><input
                v-model="draft.groups"
                type="checkbox"
                :value="group.id"
              />{{ group.name }}</label
            >
          </div>
        </fieldset>
        <label class="block"
          ><span class="input-label">{{ p('notes') }}</span
          ><textarea v-model="draft.notes" class="input" rows="2" />
        </label>
      </form>
      <template #footer
        ><div class="flex justify-end gap-2">
          <button
            class="btn btn-secondary"
            :disabled="saving"
            @click="closeEditor"
          >
            {{ p('close') }}</button
          ><button
            class="btn btn-primary"
            form="chatgpt-account-form"
            type="submit"
            :disabled="saving"
          >
            {{ saving ? p('saving') : p('save') }}
          </button>
        </div></template
      >
    </BaseDialog>

    <BaseDialog
      :show="showOperation"
      :title="operationTitle || p('results')"
      width="wide"
      @close="showOperation = false"
    >
      <div class="space-y-3" aria-live="polite">
        <p class="text-sm">
          {{
            p('progress', {
              processed,
              total: operationTotal,
              success: operationItems.filter((item) => item.success).length,
              failed: operationItems.filter((item) => !item.success).length
            })
          }}
        </p>
        <progress
          class="h-2 w-full accent-primary-600"
          :value="processed"
          :max="operationTotal || 1"
        />
        <p v-if="cancelled" class="text-xs text-amber-600">
          {{ p('cancelled') }}
        </p>
        <div
          class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-700"
        >
          <div
            v-for="(item, index) in operationItems"
            :key="index"
            class="flex items-start gap-3 border-b border-gray-100 p-3 text-xs dark:border-dark-700"
          >
            <span class="font-mono text-gray-500">#{{ item.account_id }}</span
            ><span
              class="min-w-0 flex-1 break-words"
              :class="item.success ? 'text-emerald-600' : 'text-red-600'"
              >{{ item.error || p('success') }}</span
            >
          </div>
        </div>
      </div>
      <template #footer
        ><div class="flex justify-end gap-2">
          <button
            v-if="busy"
            class="btn btn-secondary"
            @click="cancelOperation"
          >
            {{ p('cancelRemaining') }}</button
          ><button class="btn btn-primary" @click="showOperation = false">
            {{ p('close') }}
          </button>
        </div></template
      >
    </BaseDialog>

    <ConfirmDialog
      :show="pendingDelete.length > 0"
      :title="p('delete')"
      :message="p('confirmDelete', { count: pendingDelete.length })"
      :confirm-text="p('delete')"
      danger
      @confirm="confirmDelete"
      @cancel="pendingDelete = []"
    />
    <BaseDialog
      :show="showGroups"
      :title="p('manageGroups')"
      @close="showGroups = false"
      ><form class="mb-4 flex gap-2" @submit.prevent="saveGroup">
        <input
          v-model.trim="groupName"
          class="input"
          :placeholder="p('groupName')"
          required
          maxlength="100"
        /><button class="btn btn-primary" :disabled="savingGroup">
          {{ p('add') }}
        </button>
      </form>
      <div
        v-for="group in groups"
        :key="group.id"
        class="flex items-center justify-between border-b border-gray-100 py-3 text-sm dark:border-dark-700"
      >
        <span>{{ group.name }}</span>
        <div class="flex gap-3">
          <button
            class="text-primary-600"
            :disabled="savingGroup"
            @click="renameGroup(group)"
          >
            {{ p('edit') }}</button
          ><button
            class="text-red-600"
            :disabled="savingGroup"
            @click="deleteGroup(group)"
          >
            {{ p('delete') }}
          </button>
        </div>
      </div></BaseDialog
    >
    <BaseDialog
      :show="showBindGroups"
      :title="p('bindGroups')"
      @close="showBindGroups = false"
      ><p class="mb-3 text-sm text-gray-500">{{ p('bindingHint') }}</p>
      <label
        v-for="group in groups"
        :key="group.id"
        class="mb-2 flex items-center gap-2 text-sm"
        ><input v-model="bindingGroups" type="checkbox" :value="group.id" />{{
          group.name
        }}</label
      ><template #footer
        ><button class="btn btn-primary" :disabled="!!busy" @click="bindGroups">
          {{ p('save') }}
        </button></template
      ></BaseDialog
    >
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { apiClient } from '@/api/client'
import { adminAPI } from '@/api/admin'
import {
  chatgptAccountsAPI,
  type ChatGPTAccount,
  type ChatGPTOperationItem
} from '@/api/admin/chatgptAccounts'
import type {
  AdminDataPayload,
  AdminGroup,
  CodexSessionImportResult,
  Proxy
} from '@/types'
import { useAppStore } from '@/stores/app'
import {
  isStepUpBlocked,
  isStepUpCancelled,
  useStepUp
} from '@/composables/useStepUp'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import ChatGPTAccountItem from '@/components/admin/account/ChatGPTAccountItem.vue'

const { t } = useI18n()
const p = (key: string, values: Record<string, string | number> = {}) =>
  t(`admin.chatgptAccounts.${key}`, values)
const app = useAppStore()
const route = useRoute()
const stepUp = useStepUp()
const accounts = ref<ChatGPTAccount[]>([])
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const filters = reactive({ search: '', status: '', group: '', source: '' })
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const counts = ref<Record<string, number>>({})
const loading = ref(false)
const loadError = ref('')
const lastLoaded = ref('')
const selected = ref(new Set<number>())
const selecting = ref(false)
const autoRefresh = ref(false)
const viewModes = ['list', 'cards'] as const
function initialViewMode(): 'list' | 'cards' {
  try {
    return localStorage.getItem('chatgpt-account-view') === 'cards'
      ? 'cards'
      : 'list'
  } catch {
    return 'list'
  }
}
const viewMode = ref(initialViewMode())
function setViewMode(mode: 'list' | 'cards') {
  viewMode.value = mode
  try {
    localStorage.setItem('chatgpt-account-view', mode)
  } catch {
    /* storage can be unavailable */
  }
}
const statusOptions = computed(() => [
  { value: '', label: p('allStatus') },
  ...[
    'normal',
    'pending',
    'limited',
    'abnormal',
    'recoverable',
    'disabled'
  ].map((value) => ({ value, label: p(`status.${value}`) }))
])
const sourceOptions = computed(() => [
  { value: '', label: p('allSources') },
  { value: 'web', label: 'ChatGPT Web' },
  { value: 'codex', label: 'Codex' }
])
const groupOptions = computed(() => [
  { value: '', label: p('allGroups') },
  { value: 'ungrouped', label: p('ungrouped') },
  ...groups.value.map((group) => ({
    value: String(group.id),
    label: group.name
  }))
])
const bindingGroupOptions = computed(() => [
  { value: '', label: p('ungrouped') },
  ...groups.value.map((group) => ({
    value: String(group.id),
    label: group.name
  }))
])
const proxyOptions = computed(() => [
  { value: '', label: p('direct') },
  ...proxies.value.map((proxy) => ({
    value: String(proxy.id),
    label: proxy.name
  }))
])
const bulkAction = ref('')
const actionOptions = computed(() => [
  { value: '', label: p('bulkActions') },
  ...['sync', 'refresh', 'enable', 'disable', 'reset', 'bind', 'delete'].map(
    (value) => ({ value, label: p(`bulk.${value}`) })
  )
])
const exportFormat = ref('')
const exportOptions = computed(() => [
  { value: '', label: p('export') },
  ...['selected_json', 'selected_txt', 'all_json', 'all_txt'].map((value) => ({
    value,
    label: p(`exports.${value}`)
  }))
])
const allPageSelected = computed(
  () =>
    accounts.value.length > 0 &&
    accounts.value.every((account) => selected.value.has(account.id))
)
const somePageSelected = computed(
  () =>
    !allPageSelected.value &&
    accounts.value.some((account) => selected.value.has(account.id))
)
const accountGroups = (account: ChatGPTAccount) =>
  groups.value.filter((group) => account.group_ids?.includes(group.id))
function toggleSelect(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
}
function selectPage(checked: boolean) {
  const next = new Set(selected.value)
  for (const account of accounts.value) {
    if (checked) next.add(account.id)
    else next.delete(account.id)
  }
  selected.value = next
}
function errorMessage(error: unknown) {
  return error && typeof error === 'object' && 'message' in error
    ? String(error.message)
    : p('requestFailed')
}
let disposed = false
let loadController: AbortController | null = null
let loadVersion = 0
let selectionVersion = 0
let filterTimer: ReturnType<typeof setTimeout> | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null
async function load() {
  const version = ++loadVersion
  loadController?.abort()
  loadController = new AbortController()
  loading.value = true
  try {
    const data = await chatgptAccountsAPI.list(
      page.value,
      pageSize.value,
      { ...filters },
      loadController.signal
    )
    if (disposed || version !== loadVersion) return
    accounts.value = data.items
    total.value = data.total
    counts.value = data.counts
    loadError.value = ''
    lastLoaded.value = new Date().toISOString()
    if (page.value > 1 && !data.items.length && data.total > 0) {
      page.value = Math.max(1, data.pages)
      void load()
    }
  } catch (error) {
    if (!disposed && version === loadVersion && !loadController?.signal.aborted)
      loadError.value = errorMessage(error)
  } finally {
    if (version === loadVersion) loading.value = false
  }
}
watch(filters, () => {
  selectionVersion++
  selecting.value = false
  selected.value = new Set()
  page.value = 1
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    void load()
  }, 250)
})
function changePage(value: number) {
  page.value = value
  void load()
}
function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
  void load()
}
async function selectAll() {
  const version = ++selectionVersion
  selecting.value = true
  try {
    const ids = await chatgptAccountsAPI.ids({ ...filters })
    if (version === selectionVersion && !disposed) selected.value = new Set(ids)
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    if (version === selectionVersion) selecting.value = false
  }
}

const busy = ref('')
const operationItems = ref<ChatGPTOperationItem[]>([])
const operationTitle = ref('')
const operationTotal = ref(0)
const processed = ref(0)
const cancelled = ref(false)
const showOperation = ref(false)
let operationController: AbortController | null = null
function cancelOperation() {
  cancelled.value = true
}
async function runOperation(action: string, targetIds: number[]) {
  if (busy.value || !targetIds.length) return
  const ids = [...new Set(targetIds)]
  busy.value = action
  operationTitle.value = p(`bulk.${action}`)
  operationTotal.value = ids.length
  processed.value = 0
  operationItems.value = []
  cancelled.value = false
  showOperation.value = true
  operationController = new AbortController()
  const names = new Map(
    accounts.value.map((account) => [account.id, account.name])
  )
  try {
    for (
      let index = 0;
      index < ids.length && !cancelled.value && !disposed;
      index += 5
    ) {
      const chunk = ids.slice(index, index + 5)
      let items: ChatGPTOperationItem[]
      if (action === 'sync' || action === 'refresh') {
        const result = await chatgptAccountsAPI.maintain(
          chunk,
          action,
          operationController.signal
        )
        items = result.items
      } else if (action === 'delete' || action === 'reset') {
        const result =
          action === 'delete'
            ? await adminAPI.accounts.batchDelete(chunk)
            : await adminAPI.accounts.batchClearError(chunk)
        const failures = new Map(
          result.errors?.map((item) => [item.account_id, item.error]) || []
        )
        const successIDs = new Set(result.success_ids || [])
        const completeErrors =
          result.success + result.failed === chunk.length &&
          failures.size === result.failed
        items = chunk.map((id) => ({
          account_id: id,
          success: successIDs.has(id) || (completeErrors && !failures.has(id)),
          error:
            failures.get(id) ||
            (!completeErrors && !successIDs.has(id)
              ? p('unknownResult')
              : undefined)
        }))
      } else {
        const enabled = action === 'enable'
        const result = await adminAPI.accounts.bulkUpdate(
          chunk,
          action === 'bind'
            ? { group_ids: [...bindingGroups.value] }
            : { status: enabled ? 'active' : 'inactive', schedulable: enabled }
        )
        items =
          result.results?.map((item) => ({
            account_id: item.account_id,
            success: item.success,
            error: item.error
          })) || []
        if (!items.length) {
          const successIDs = new Set(result.success_ids || [])
          items = chunk.map((id) => ({
            account_id: id,
            success: successIDs.has(id),
            error: successIDs.has(id) ? undefined : p('unknownResult')
          }))
        }
      }
      operationItems.value.push(...items)
      processed.value += items.length
      const next = new Set(selected.value)
      for (const item of items) {
        if (item.success) next.delete(item.account_id)
        if (item.account && names.has(item.account_id)) {
          const row = accounts.value.findIndex(
            (account) => account.id === item.account_id
          )
          if (row >= 0) accounts.value[row] = item.account
        }
      }
      selected.value = next
    }
  } catch (error) {
    if (!cancelled.value && !disposed) app.showError(errorMessage(error))
  } finally {
    busy.value = ''
    operationController = null
    if (!disposed) await load()
  }
}
const pendingDelete = ref<number[]>([])
function requestDelete(ids: number[]) {
  pendingDelete.value = [...ids]
}
function confirmDelete() {
  const ids = [...pendingDelete.value]
  pendingDelete.value = []
  void runOperation('delete', ids)
}
function onBulkAction() {
  const action = bulkAction.value
  bulkAction.value = ''
  if (!action) return
  if (action === 'delete') requestDelete([...selected.value])
  else if (action === 'bind') {
    bindingGroups.value = []
    showBindGroups.value = true
  } else void runOperation(action, [...selected.value])
}

const showImport = ref(false)
const importing = ref(false)
const importContent = ref('')
const importFiles = ref<File[]>([])
const importSource = ref<'web' | 'codex'>('web')
const importGroup = ref('')
const importProxy = ref('')
const updateExisting = ref(false)
const importResult = ref<CodexSessionImportResult | null>(null)
const oauthLoading = ref(false)
const oauthUrl = ref('')
const oauthSession = ref('')
const oauthCallback = ref('')
const remoteKind = ref<'cpa' | 'sub2api'>('cpa')
const remoteURL = ref('')
const remoteKey = ref('')
async function importRemote() {
  if (importing.value) return
  importing.value = true
  try {
    importResult.value = await chatgptAccountsAPI.importRemote({
      kind: remoteKind.value,
      base_url: remoteURL.value,
      api_key: remoteKey.value,
      source: importSource.value,
      group_ids: importGroup.value ? [Number(importGroup.value)] : undefined,
      proxy_id: importProxy.value ? Number(importProxy.value) : undefined,
      update_existing: updateExisting.value
    })
    remoteKey.value = ''
    await load()
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    importing.value = false
  }
}
function openImport() {
  showImport.value = true
  importResult.value = null
}
function closeImport() {
  if (importing.value) return
  showImport.value = false
  importContent.value = ''
  importFiles.value = []
  oauthCallback.value = ''
  oauthUrl.value = ''
  oauthSession.value = ''
  remoteKey.value = ''
}
function chooseFiles(event: Event) {
  importFiles.value = Array.from((event.target as HTMLInputElement).files || [])
}
async function importAccounts() {
  if (importing.value) return
  if (
    importFiles.value.reduce(
      (size, file) => size + file.size,
      new Blob([importContent.value]).size
    ) >
    12 * 1024 * 1024
  ) {
    app.showError(p('importSizeLimit'))
    return
  }
  importing.value = true
  try {
    const contents = await Promise.all(
      importFiles.value.map((file) => file.text())
    )
    const documents = [importContent.value, ...contents].filter((value) =>
      value.trim()
    )
    const backups: AdminDataPayload[] = []
    const tokens: string[] = []
    for (const content of documents) {
      let parsed: unknown
      try {
        parsed = JSON.parse(content)
      } catch {
        tokens.push(content)
        continue
      }
      if (
        parsed &&
        typeof parsed === 'object' &&
        'accounts' in parsed &&
        'proxies' in parsed &&
        Array.isArray(parsed.accounts) &&
        Array.isArray(parsed.proxies)
      ) {
        const backup = parsed as AdminDataPayload
        if (backup.accounts.length > 1000)
          throw new Error(p('importCountLimit'))
        if (backup.accounts.some((account) => account.platform !== 'chatgpt'))
          throw new Error(p('backupPoolOnly'))
        backups.push(backup)
      } else tokens.push(content)
    }
    const result: CodexSessionImportResult = {
      total: 0,
      created: 0,
      updated: 0,
      skipped: 0,
      failed: 0,
      errors: [],
      warnings: []
    }
    for (const backup of backups) {
      const restored = await adminAPI.accounts.importData({
        data: backup,
        skip_default_group_bind: true
      })
      result.total += backup.accounts.length
      result.created += restored.account_created
      result.failed += restored.account_failed + restored.proxy_failed
      result.errors!.push(
        ...(restored.errors || []).map((item, index) => ({
          index: index + 1,
          name: item.name,
          message: item.message
        }))
      )
    }
    if (tokens.length) {
      const imported = await chatgptAccountsAPI.import({
        contents: tokens,
        source: importSource.value,
        group_ids: importGroup.value ? [Number(importGroup.value)] : undefined,
        proxy_id: importProxy.value ? Number(importProxy.value) : undefined,
        update_existing: updateExisting.value
      })
      result.total += imported.total
      result.created += imported.created
      result.updated += imported.updated
      result.skipped += imported.skipped
      result.failed += imported.failed
      result.errors!.push(...(imported.errors || []))
      result.warnings!.push(...(imported.warnings || []))
    }
    importResult.value = result
    if (backups.length) await loadGroups()
    if (importResult.value.created || importResult.value.updated) {
      importContent.value = ''
      importFiles.value = []
      await load()
    }
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    importing.value = false
  }
}
async function generateOAuth() {
  oauthLoading.value = true
  try {
    const { data } = await apiClient.post<{
      auth_url: string
      session_id: string
    }>('/admin/openai/generate-auth-url', {
      proxy_id: importProxy.value ? Number(importProxy.value) : undefined
    })
    oauthUrl.value = data.auth_url
    oauthSession.value = data.session_id
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    oauthLoading.value = false
  }
}
async function exchangeOAuth() {
  oauthLoading.value = true
  try {
    const callback = new URL(oauthCallback.value.trim())
    const code = callback.searchParams.get('code'),
      state = callback.searchParams.get('state')
    if (!code || !state) throw new Error(p('invalidCallback'))
    const { data } = await apiClient.post<Record<string, unknown>>(
      '/admin/openai/exchange-code',
      {
        code,
        state,
        session_id: oauthSession.value,
        proxy_id: importProxy.value ? Number(importProxy.value) : undefined
      }
    )
    importContent.value = JSON.stringify(data)
    oauthCallback.value = ''
    oauthSession.value = ''
    oauthUrl.value = ''
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    oauthLoading.value = false
  }
}

const showEditor = ref(false)
const saving = ref(false)
const editing = ref<ChatGPTAccount | null>(null)
const draft = reactive({
  name: '',
  email: '',
  plan: '',
  access: '',
  refresh: '',
  source: 'web',
  proxy: '',
  groups: [] as number[],
  notes: ''
})
function openEditor(account: ChatGPTAccount | null) {
  editing.value = account
  Object.assign(draft, {
    name: account?.name || '',
    email: String(account?.credentials?.email || ''),
    plan: String(account?.credentials?.plan_type || ''),
    access: '',
    refresh: '',
    source: String(account?.credentials?.source_type || 'web'),
    proxy: account?.proxy_id ? String(account.proxy_id) : '',
    groups: [...(account?.group_ids || [])],
    notes: account?.notes || ''
  })
  showEditor.value = true
}
function closeEditor() {
  if (saving.value) return
  showEditor.value = false
  draft.access = ''
  draft.refresh = ''
  editing.value = null
}
async function saveAccount() {
  if (saving.value) return
  saving.value = true
  try {
    const credentials: Record<string, unknown> = {
      ...editing.value?.credentials,
      email: draft.email,
      plan_type: draft.plan,
      source_type: draft.source
    }
    if (draft.access) credentials.access_token = draft.access
    if (draft.refresh) credentials.refresh_token = draft.refresh
    const changes = {
      name: draft.name,
      notes: draft.notes,
      credentials,
      proxy_id: draft.proxy ? Number(draft.proxy) : 0,
      group_ids: [...draft.groups]
    }
    if (editing.value) await adminAPI.accounts.update(editing.value.id, changes)
    else {
      const result = await chatgptAccountsAPI.import({
        name: draft.name,
        source: draft.source as 'web' | 'codex',
        content: JSON.stringify({
          ...credentials,
          name: draft.name,
          notes: draft.notes
        }),
        proxy_id: draft.proxy ? Number(draft.proxy) : undefined,
        group_ids: [...draft.groups],
        update_existing: false
      })
      if (!result.created && !result.updated)
        throw new Error(
          result.errors?.[0]?.message || p('accountAlreadyExists')
        )
    }
    saving.value = false
    closeEditor()
    await load()
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    saving.value = false
  }
}

async function copyCredential(
  account: ChatGPTAccount,
  kind: 'access_token' | 'refresh_token'
) {
  try {
    const token = await stepUp.run(() =>
      chatgptAccountsAPI.credential(account.id, kind)
    )
    if (!token) {
      app.showError(p('credentialMissing'))
      return
    }
    await navigator.clipboard.writeText(token)
    app.showSuccess(p('copied'))
  } catch (error) {
    handleSensitiveError(error)
  }
}
function handleSensitiveError(error: unknown) {
  if (isStepUpCancelled(error)) return
  app.showError(
    isStepUpBlocked(error) ? t('stepUp.notEnabled') : errorMessage(error)
  )
}
async function onExport() {
  const format = exportFormat.value
  exportFormat.value = ''
  if (!format) return
  try {
    const ids = format.startsWith('selected')
      ? [...selected.value]
      : await chatgptAccountsAPI.ids({ ...filters })
    if (!ids.length) {
      app.showError(p('selectFirst'))
      return
    }
    const result = await stepUp.run(async () => {
      const payload: AdminDataPayload = {
        exported_at: new Date().toISOString(),
        accounts: [],
        proxies: []
      }
      const seenProxies = new Set<string>()
      for (let index = 0; index < ids.length; index += 500) {
        const chunk = await adminAPI.accounts.exportData({
          ids: ids.slice(index, index + 500),
          includeProxies: true
        })
        payload.accounts.push(...chunk.accounts)
        for (const proxy of chunk.proxies) {
          if (!seenProxies.has(proxy.proxy_key)) {
            seenProxies.add(proxy.proxy_key)
            payload.proxies.push(proxy)
          }
        }
      }
      return payload
    })
    const txt = format.endsWith('txt')
    const content = txt
      ? result.accounts
          .map((account) => account.credentials.access_token)
          .filter(
            (token): token is string => typeof token === 'string' && !!token
          )
          .join('\n')
      : JSON.stringify(result, null, 2)
    const blob = new Blob([content], {
      type: txt ? 'text/plain;charset=utf-8' : 'application/json'
    })
    const url = URL.createObjectURL(blob),
      anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `chatgpt-accounts-${new Date().toISOString().replace(/[:.]/g, '-')}.${txt ? 'txt' : 'json'}`
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (error) {
    handleSensitiveError(error)
  }
}

const showGroups = ref(false)
const groupName = ref('')
const savingGroup = ref(false)
const showBindGroups = ref(false)
const bindingGroups = ref<number[]>([])
async function loadGroups() {
  groups.value = await adminAPI.groups.getAll('chatgpt')
}
async function saveGroup() {
  if (savingGroup.value) return
  savingGroup.value = true
  try {
    await adminAPI.groups.create({
      name: groupName.value,
      platform: 'chatgpt',
      rate_multiplier: 1,
      subscription_type: 'standard'
    })
    groupName.value = ''
    await loadGroups()
  } catch (error) {
    app.showError(errorMessage(error))
  } finally {
    savingGroup.value = false
  }
}
async function renameGroup(group: AdminGroup) {
  const name = window.prompt(p('groupName'), group.name)?.trim()
  if (!name || name === group.name) return
  try {
    await adminAPI.groups.update(group.id, { name })
    await loadGroups()
  } catch (error) {
    app.showError(errorMessage(error))
  }
}
async function deleteGroup(group: AdminGroup) {
  if (!window.confirm(p('confirmDeleteGroup', { name: group.name }))) return
  try {
    await adminAPI.groups.delete(group.id)
    await loadGroups()
    await load()
  } catch (error) {
    app.showError(errorMessage(error))
  }
}
async function bindGroups() {
  showBindGroups.value = false
  await runOperation('bind', [...selected.value])
}

onMounted(async () => {
  const results = await Promise.allSettled([
    load(),
    loadGroups(),
    adminAPI.proxies.getAll().then((result) => {
      proxies.value = result
    })
  ])
  for (const result of results)
    if (result.status === 'rejected') app.showError(errorMessage(result.reason))
  if (disposed) return
  if (route.query.edit) {
    let account = accounts.value.find(
      (account) => account.id === Number(route.query.edit)
    )
    if (!account) {
      try {
        const detail = await adminAPI.accounts.getById(Number(route.query.edit))
        if (detail.platform === 'chatgpt') account = detail
      } catch (error) {
        app.showError(errorMessage(error))
      }
    }
    if (account) openEditor(account)
  }
  refreshTimer = setInterval(() => {
    if (
      autoRefresh.value &&
      document.visibilityState === 'visible' &&
      !busy.value &&
      !loading.value &&
      !showEditor.value &&
      !showImport.value
    )
      void load()
  }, 30000)
})
onUnmounted(() => {
  disposed = true
  loadController?.abort()
  operationController?.abort()
  if (filterTimer) clearTimeout(filterTimer)
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
:deep(.table-page-layout.mobile-mode) {
  height: auto;
}
:deep(.table-page-layout.mobile-mode .pool-scroll) {
  max-height: 70dvh;
}
</style>
