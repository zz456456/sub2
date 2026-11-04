import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import ChatGPTAccountsView from '../ChatGPTAccountsView.vue'
import ChatGPTAccountItem from '@/components/admin/account/ChatGPTAccountItem.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  ids: vi.fn(),
  maintain: vi.fn(),
  importAccounts: vi.fn(),
  credential: vi.fn(),
  bulkUpdate: vi.fn(),
  batchDelete: vi.fn(),
  update: vi.fn(),
  exportData: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  getGroups: vi.fn(),
  getProxies: vi.fn()
}))
vi.mock('@/api/admin/chatgptAccounts', async () => ({
  ...(await vi.importActual<typeof import('@/api/admin/chatgptAccounts')>(
    '@/api/admin/chatgptAccounts'
  )),
  chatgptAccountsAPI: {
    list: mocks.list,
    ids: mocks.ids,
    maintain: mocks.maintain,
    import: mocks.importAccounts,
    credential: mocks.credential
  }
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      bulkUpdate: mocks.bulkUpdate,
      batchDelete: mocks.batchDelete,
      batchClearError: vi.fn(),
      update: mocks.update,
      exportData: mocks.exportData
    },
    groups: {
      getAll: mocks.getGroups,
      create: vi.fn(),
      update: vi.fn(),
      delete: vi.fn()
    },
    proxies: { getAll: mocks.getProxies }
  }
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess
  })
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (fn: () => unknown) => fn() }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false
}))

const row = {
  id: 1,
  name: 'demo',
  platform: 'chatgpt',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  credentials: { email: 'demo@example.com', source_type: 'web' },
  group_ids: [7],
  extra: {},
  created_at: '2026-11-04T00:00:00Z',
  credential_state: {
    access: 'expiring',
    refresh: 'present',
    can_refresh: true
  },
  chatgpt_pool_state: { status: 'pending', quota: 'unknown' }
}
const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template:
    '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>'
})
function mountView() {
  return mount(ChatGPTAccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template:
            '<div><slot name="filters"/><slot name="table"/><slot name="pagination"/></div>'
        },
        ChatGPTAccountItem: true,
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/><slot name="footer"/></div>'
        },
        Select: SelectStub,
        SearchInput: true,
        ConfirmDialog: true,
        Pagination: true,
        TotpStepUpDialog: true,
        Icon: true
      }
    }
  })
}
type Wrapper = ReturnType<typeof mountView>
const wrappers: Wrapper[] = []
async function createView() {
  const wrapper = mountView()
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('ChatGPT account pool', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mocks.list.mockResolvedValue({
      items: [row],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1,
      counts: { pending: 2 }
    })
    mocks.ids.mockResolvedValue([1, 2])
    mocks.getGroups.mockResolvedValue([
      { id: 7, name: 'images', platform: 'chatgpt' }
    ])
    mocks.getProxies.mockResolvedValue([])
  })
  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.useRealTimers()
  })

  it('loads the dedicated pool and binds group names without fetching raw credentials', async () => {
    const wrapper = await createView()
    expect(mocks.list).toHaveBeenCalledWith(
      1,
      20,
      { search: '', status: '', group: '', source: '' },
      expect.any(AbortSignal)
    )
    expect(wrapper.findComponent(ChatGPTAccountItem).props('groups')).toEqual([
      { id: 7, name: 'images', platform: 'chatgpt' }
    ])
    expect(mocks.credential).not.toHaveBeenCalled()
  })
  it('persists card view while retaining the current page selection', async () => {
    const wrapper = await createView()
    wrapper.findComponent(ChatGPTAccountItem).vm.$emit('select', 1)
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'admin.chatgptAccounts.cards')!
      .trigger('click')
    expect(localStorage.getItem('chatgpt-account-view')).toBe('cards')
    expect(wrapper.findComponent(ChatGPTAccountItem).props()).toMatchObject({
      layout: 'cards',
      selected: true
    })
  })
  it('retains only failed targets after a partial batch refresh and exposes the result', async () => {
    const wrapper = await createView()
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'admin.chatgptAccounts.selectAll')!
      .trigger('click')
    await flushPromises()
    mocks.maintain.mockResolvedValue({
      total: 2,
      success: 1,
      failed: 1,
      items: [
        { account_id: 1, success: true },
        { account_id: 2, success: false, error: 'ChatGPT HTTP 401' }
      ]
    })
    const actions = wrapper
      .findAll('select')
      .find(
        (select) =>
          select.attributes('aria-label') ===
          'admin.chatgptAccounts.bulkActions'
      )!
    await actions.setValue('refresh')
    await flushPromises()
    expect(mocks.maintain).toHaveBeenCalledWith(
      [1, 2],
      'refresh',
      expect.any(AbortSignal)
    )
    expect(wrapper.text()).toContain('ChatGPT HTTP 401')
    expect(wrapper.findComponent(ChatGPTAccountItem).props('selected')).toBe(
      false
    )
  })
  it('keeps a previous snapshot visible when a background reload fails', async () => {
    const wrapper = await createView()
    mocks.list.mockRejectedValue({ message: 'offline' })
    await wrapper
      .find('button[aria-label="admin.chatgptAccounts.reload"]')
      .trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('offline')
    expect(wrapper.findComponent(ChatGPTAccountItem).exists()).toBe(true)
  })
  it('reports a failed create result and leaves the credential draft available for correction', async () => {
    const wrapper = await createView()
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'admin.chatgptAccounts.add')!
      .trigger('click')
    const form = wrapper.find('#chatgpt-account-form')
    await form.find('input').setValue('new account')
    await form.find('textarea').setValue('token')
    mocks.importAccounts.mockResolvedValue({
      total: 1,
      created: 0,
      updated: 0,
      skipped: 0,
      failed: 1,
      errors: [{ index: 1, message: 'invalid account' }]
    })
    await form.trigger('submit')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('invalid account')
    expect(wrapper.find('#chatgpt-account-form').exists()).toBe(true)
  })
})
