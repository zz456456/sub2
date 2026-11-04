import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import VersionBadge from '../common/VersionBadge.vue'

const mocks = vi.hoisted(() => ({
  auth: { isAdmin: true },
  checkUpdates: vi.fn(),
  copy: vi.fn()
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/api/admin/system', () => ({ checkUpdates: mocks.checkUpdates }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: mocks.copy })
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const release = {
  current_version: '0.2.19',
  latest_version: '0.2.20',
  has_update: true,
  cached: false,
  build_type: 'release',
  update_repository: 'zz456456/sub2',
  docker_image: 'ghcr.io/zz456456/sub2'
}

describe('VersionBadge custom Docker releases', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.auth.isAdmin = true
    mocks.checkUpdates.mockResolvedValue({ ...release })
  })

  afterEach(() => { wrapper?.unmount() })

  async function openBadge() {
    wrapper = mount(VersionBadge, { props: { version: '0.2.19' } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    return wrapper
  }

  it('keeps regular users on static version text without update requests', async () => {
    mocks.auth.isAdmin = false
    wrapper = mount(VersionBadge, { props: { version: '0.2.19' } })
    await flushPromises()
    expect(wrapper.text()).toBe('v0.2.19')
    expect(wrapper.find('button').exists()).toBe(false)
    expect(mocks.checkUpdates).not.toHaveBeenCalled()
  })

  it('shows the fork release and copies Docker Compose update commands', async () => {
    const badge = await openBadge()
    expect(mocks.checkUpdates).toHaveBeenCalledWith(false)
    expect(badge.text()).toContain('version.updateAvailable')
    expect(badge.text()).toContain('ghcr.io/zz456456/sub2:latest')
    expect(badge.get('a').attributes('href')).toBe('https://github.com/zz456456/sub2/releases')
    const copyButton = badge.findAll('button').find((button) => button.text() === 'version.copyCommand')!
    await copyButton.trigger('click')
    expect(mocks.copy).toHaveBeenCalledWith('docker compose pull sub2api\ndocker compose up -d --no-deps sub2api')
  })

  it('forces a fresh release check when refreshed', async () => {
    const badge = await openBadge()
    const refreshButton = badge.findAll('button').find((button) => button.text() === 'version.refresh')!
    await refreshButton.trigger('click')
    await flushPromises()
    expect(mocks.checkUpdates).toHaveBeenLastCalledWith(true)
  })

  it('explains that the first custom release has not been published', async () => {
    mocks.checkUpdates.mockResolvedValue({ ...release, warning: 'GitHub API returned 404' })
    const badge = await openBadge()
    expect(badge.text()).toContain('version.noCustomRelease')
    expect(badge.text()).not.toContain('version.upToDate')
    expect(badge.find('pre').exists()).toBe(false)
  })

  it('keeps a failed check distinct from being up to date', async () => {
    mocks.checkUpdates.mockRejectedValue(new Error('Network unavailable'))
    const badge = await openBadge()
    expect(badge.text()).toContain('version.checkFailed')
    expect(badge.text()).not.toContain('version.upToDate')
    expect(badge.find('pre').exists()).toBe(false)
  })
})
