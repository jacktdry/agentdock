import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  Availability,
  Domain,
  clientError,
  desktopApi,
  type APIError,
  type DomainCapability,
  type Manifest,
} from '../api/desktopApi'
import { t } from '../i18n'

export const useContractStore = defineStore('contract', () => {
  const manifest = ref<Manifest | null>(null)
  const negotiated = ref<DomainCapability[]>([])
  const loading = ref(false)
  const error = ref<APIError | null>(null)
  let loadPromise: Promise<void> | null = null

  const unavailableCount = computed(
    () =>
      (manifest.value?.capabilities ?? []).filter(
        (capability) => capability.availability === Availability.AvailabilityUnavailable,
      ).length,
  )

  const experimentalCount = computed(
    () =>
      (manifest.value?.capabilities ?? []).filter(
        (capability) => capability.availability === Availability.AvailabilityExperimental,
      ).length,
  )

  async function loadNow() {
    loading.value = true
    error.value = null
    try {
      const nextManifest = await desktopApi.manifest()
      manifest.value = nextManifest
      const domains = (nextManifest.capabilities ?? []).map((capability) => capability.domain)
      const result = await desktopApi.negotiate(domains)
      if (!result.accepted || result.error) {
        error.value =
          result.error ?? clientError('desktop_api_negotiation_rejected', t('common.rejected'))
        negotiated.value = []
        return
      }
      negotiated.value = result.capabilities ?? []
    } catch (caught) {
      error.value = clientError('desktop_api_negotiation_failed', caught)
      negotiated.value = []
    } finally {
      loading.value = false
    }
  }

  function load() {
    if (manifest.value && negotiated.value.length > 0 && !error.value) {
      return Promise.resolve()
    }
    if (!loadPromise) {
      loadPromise = loadNow().finally(() => {
        loadPromise = null
      })
    }
    return loadPromise
  }

  function operationCapability(domain: Domain, name: string) {
    return negotiated.value
      .find((capability) => capability.domain === domain)
      ?.operations?.find((operation) => operation.name === name)
  }

  function canInvoke(domain: Domain, name: string) {
    return operationCapability(domain, name) !== undefined
  }

  function requiresConfirmation(domain: Domain, name: string) {
    return operationCapability(domain, name)?.requiresConfirmation ?? true
  }

  return {
    manifest,
    negotiated,
    loading,
    error,
    unavailableCount,
    experimentalCount,
    load,
    canInvoke,
    requiresConfirmation,
  }
})
