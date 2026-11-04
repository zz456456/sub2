import { apiClient } from '../client'
import type {
  AccountListItem,
  CodexSessionImportResult,
  PaginatedResponse
} from '@/types'

export interface CredentialState {
  access:
    | 'missing'
    | 'inherited'
    | 'unknown'
    | 'expired'
    | 'expiring'
    | 'valid'
    | 'invalid'
  refresh: 'missing' | 'inherited' | 'present' | 'invalid'
  issued_at?: number
  expires_at?: number
  can_refresh: boolean
}

export interface ChatGPTSnapshot {
  email?: string
  user_id?: string
  account_id?: string
  plan?: string
  display_name?: string
  quota_remaining: number | null
  quota_reset_at?: string
  deactivated: boolean
  synced_at: string
}

export type ChatGPTAccount = AccountListItem & {
  credential_state?: CredentialState
  chatgpt_pool_state?: {
    status:
      'normal' | 'pending' | 'limited' | 'abnormal' | 'recoverable' | 'disabled'
    quota: 'unknown' | 'available' | 'exhausted' | 'unlimited'
  }
}
export interface ChatGPTOperationItem {
  account_id: number
  success: boolean
  error?: string
  account?: ChatGPTAccount
}
export interface ChatGPTOperationResult {
  total: number
  success: number
  failed: number
  items: ChatGPTOperationItem[]
}

export const chatgptAccountsAPI = {
  async importRemote(payload: {
    kind: 'cpa' | 'sub2api'
    base_url: string
    api_key: string
    source: 'web' | 'codex'
    group_ids?: number[]
    proxy_id?: number
    update_existing: boolean
  }) {
    const { data } = await apiClient.post<CodexSessionImportResult>(
      '/admin/accounts/chatgpt/import-remote',
      payload,
      { timeout: 120000 }
    )
    return data
  },
  async list(
    page: number,
    pageSize: number,
    filters: {
      search?: string
      status?: string
      group?: string
      source?: string
    },
    signal?: AbortSignal
  ) {
    const { data } = await apiClient.get<
      PaginatedResponse<ChatGPTAccount> & { counts: Record<string, number> }
    >('/admin/accounts/chatgpt', {
      params: { page, page_size: pageSize, ...filters },
      signal
    })
    return data
  },
  async ids(filters: {
    search?: string
    status?: string
    group?: string
    source?: string
  }) {
    const { data } = await apiClient.get<{ ids: number[] }>(
      '/admin/accounts/chatgpt',
      { params: { ...filters, ids_only: '1' } }
    )
    return data.ids
  },
  async import(payload: {
    content?: string
    contents?: string[]
    name?: string
    source: 'web' | 'codex'
    group_ids?: number[]
    proxy_id?: number | null
    update_existing: boolean
  }) {
    const { data } = await apiClient.post<CodexSessionImportResult>(
      '/admin/accounts/chatgpt/import',
      payload,
      { timeout: 120000 }
    )
    return data
  },
  async maintain(
    accountIds: number[],
    action: 'sync' | 'refresh',
    signal?: AbortSignal
  ) {
    const { data } = await apiClient.post<ChatGPTOperationResult>(
      '/admin/accounts/chatgpt/maintain',
      {
        account_ids: accountIds,
        action
      },
      { timeout: 120000, signal }
    )
    return data
  },
  async credential(id: number, kind: 'access_token' | 'refresh_token') {
    const { data } = await apiClient.get<{ token: string }>(
      `/admin/accounts/${id}/chatgpt-credential/${kind}`
    )
    return data.token
  }
}

export function chatgptSnapshot(
  account: ChatGPTAccount
): ChatGPTSnapshot | null {
  const value: unknown = account.extra?.chatgpt_snapshot
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as ChatGPTSnapshot
}
