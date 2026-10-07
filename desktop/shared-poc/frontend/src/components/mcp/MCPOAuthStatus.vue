<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { MCPManagedServer } from '../../api/desktopApi'
import { useMCPStore } from '../../stores/mcp'
import { useI18n, type MessageKey } from '../../i18n'
import ConfirmDialog from '../common/ConfirmDialog.vue'

const props = defineProps<{ server: MCPManagedServer; registryRevision: string }>()
const store = useMCPStore()
const { t } = useI18n()
const clearOpen = ref(false)

const status = computed(() => store.authorization?.status || props.server.observation.authStatus || 'unknown')
const knownStatuses = new Set([
  'unknown',
  'not_applicable',
  'unauthorized',
  'auth_required',
  'authorizing',
  'authorized',
  'denied',
  'expired',
  'failed',
  'cancelled',
  'callback_received',
  'finishing',
])

const statusLabel = computed(() => {
  const normalized = knownStatuses.has(status.value) ? status.value : 'unknown'
  return t((`mcp.oauth_status_${normalized}`) as MessageKey)
})
const reconnectPending = computed(() => status.value === 'authorized' && props.server.observation.status !== 'ready')

async function begin(callbackID = '') {
  await store.authorize(props.server, callbackID, props.registryRevision, props.server.generation)
}

async function confirmClear() {
  clearOpen.value = false
  await store.clearAuthorization(props.server, props.registryRevision, props.server.generation)
}

async function reload() {
  await store.readAuthorization(props.server)
}

watch(() => props.server.generation, () => void reload())
onMounted(() => void reload())
onUnmounted(() => store.stopOAuthPolling())
</script>

<template>
  <section class="mcp-subsection" aria-labelledby="mcp-oauth-title">
    <div class="section-heading">
      <h3 id="mcp-oauth-title">{{ t('mcp.oauth_title') }}</h3>
      <p class="field-hint">{{ t('mcp.oauth_hint') }}</p>
    </div>

    <div class="mcp-auth-status">
      <span class="state-pill" :data-state="status === 'authorized' ? 'healthy' : status">{{ statusLabel }}</span>
      <span v-if="store.authorization?.errorCode" class="execution-meta mono">{{ store.authorization.errorCode }}</span>
    </div>

    <p v-if="reconnectPending" class="restart-notice">{{ t('mcp.oauth_reconnect_pending') }}</p>

    <div v-if="store.callbackOptions.length" class="mcp-callback-options">
      <p>{{ t('mcp.choose_callback') }}</p>
      <div class="actions">
        <button v-for="option in store.callbackOptions" :key="option.id" type="button" :disabled="store.busy" @click="begin(option.id)">
          {{ option.label || option.id }}
        </button>
      </div>
    </div>

    <div class="actions">
      <button v-if="store.callbackOptions.length === 0" type="button" :disabled="store.busy" @click="begin()">{{ t('mcp.sign_in') }}</button>
      <button type="button" class="danger-button" :disabled="store.busy" @click="clearOpen = true">{{ t('mcp.clear_authorization') }}</button>
    </div>

    <ConfirmDialog v-if="clearOpen" :prompt="t('mcp.confirm_clear_authorization')" @cancel="clearOpen = false" @confirm="confirmClear">
      <p class="field-hint">{{ t('mcp.clear_authorization_hint') }}</p>
    </ConfirmDialog>
  </section>
</template>
