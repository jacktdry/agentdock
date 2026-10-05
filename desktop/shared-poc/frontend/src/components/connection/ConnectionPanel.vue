<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
} from 'vue'
import {
  Domain,
  OAuthPasswordState,
  PortState,
  type ConnectionActionName,
} from '../../api/desktopApi'
import {
  actionablePortObservation,
  connectorReadiness,
  namedOriginFromPublicMCPURL,
  safeMCPURL,
  validNamedOrigin,
} from '../../features/connectionLogic'
import { useConnectionStore } from '../../stores/connection'
import { useContractStore } from '../../stores/contract'
import { useI18n, type MessageKey } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
import ConfirmDialog from '../common/ConfirmDialog.vue'

type PendingConfirmation =
  | { kind: 'action'; action: ConnectionActionName }
  | { kind: 'port'; port: number }
  | { kind: 'tunnel'; mode: string; origin: string }
  | { kind: 'autostart'; enabled: boolean }

const store = useConnectionStore()
const contract = useContractStore()
const { t } = useI18n()

const actions: ConnectionActionName[] = ['start', 'stop', 'restart', 'regenerate']
const actionKeys: Record<ConnectionActionName, MessageKey> = {
  start: 'common.start',
  stop: 'common.stop',
  restart: 'common.restart',
  regenerate: 'connection.regenerate',
}
const modeKeys: Record<string, MessageKey> = {
  none: 'connection.mode_none',
  quick: 'connection.mode_quick',
  named: 'connection.mode_named',
}
const modeHelpKeys: Record<string, MessageKey> = {
  none: 'connection.mode_none_help',
  quick: 'connection.mode_quick_help',
  named: 'connection.mode_named_help',
}
const portStateKeys: Record<string, MessageKey> = {
  [PortState.PortAvailable]: 'connection.port_available',
  [PortState.PortOwnedByNext]: 'connection.port_owned',
  [PortState.PortReserved]: 'connection.port_reserved',
  [PortState.PortConflict]: 'connection.port_conflict',
  [PortState.PortUnknown]: 'connection.port_unknown',
}
const reasonKeys: Record<string, MessageKey> = {
  operation_unavailable: 'connection.reason_unavailable',
  next_identity_unavailable: 'connection.reason_identity',
  recovery_required: 'connection.reason_recovery',
  native_service_management_required: 'connection.reason_native',
  tunnel_autostart_unavailable: 'connection.reason_autostart',
  quick_only: 'connection.reason_quick_only',
  tunnel_mode_none: 'connection.reason_mode_none',
  named_manual_route_required: 'connection.reason_named_route',
  tunnel_token_unavailable: 'connection.reason_token',
  stable_reserved: 'connection.reason_stable_reserved',
  memory_reserved: 'connection.reason_memory_reserved',
  ownership_unavailable: 'connection.reason_ownership',
  process_instance_mismatch: 'connection.reason_ownership',
  foreign_listener: 'connection.reason_foreign_listener',
  invalid_request: 'connection.reason_invalid_port',
  observation_cancelled: 'connection.reason_check_cancelled',
  probe_failed: 'connection.reason_probe_failed',
  listener_changed: 'connection.reason_listener_changed',
  bind_available: 'connection.reason_bind_available',
  selected_next_process: 'connection.reason_next_owner',
}
const endpointKeys: Record<string, MessageKey> = {
  not_configured: 'connection.endpoint_not_configured',
  unchecked: 'connection.endpoint_unchecked',
  checking: 'connection.endpoint_checking',
  reachable: 'connection.endpoint_reachable',
  unreachable: 'connection.endpoint_unreachable',
  unexpected_endpoint: 'connection.endpoint_unexpected',
  stale: 'connection.endpoint_stale',
}
const passwordStateKeys: Record<string, MessageKey> = {
  [OAuthPasswordState.OAuthPasswordStored]: 'connection.password_stored',
  [OAuthPasswordState.OAuthPasswordMissing]: 'connection.password_missing',
  [OAuthPasswordState.OAuthPasswordUnreadable]: 'connection.password_unreadable',
  [OAuthPasswordState.OAuthPasswordUnavailable]: 'connection.password_unavailable',
}

const portDraft = ref(8767)
const modeDraft = ref('none')
const namedOrigin = ref('')
const tunnelToken = ref('')
const autostartDraft = ref(false)
const confirmation = shallowRef<PendingConfirmation | null>(null)
const oauthPassword = shallowRef('')
const passwordVisible = ref(false)
const copyNotice = shallowRef<'local' | 'public' | 'password' | null>(null)
const copyFailed = shallowRef(false)

let secretTimer: number | undefined
let copyTimer: number | undefined
let portTimer: number | undefined
let lastRevision = ''

const snapshot = computed(() => store.snapshot)
const localMCPURL = computed(() => safeMCPURL(snapshot.value?.localMCPURL))
const publicMCPURL = computed(() => safeMCPURL(snapshot.value?.publicMCPURL, true))
const endpointState = computed(() => store.publicEndpointState)
const readiness = computed(() => connectorReadiness(snapshot.value, endpointState.value))
const currentOrigin = computed(() => namedOriginFromPublicMCPURL(snapshot.value?.publicMCPURL))

const activePortObservation = computed(() => {
  const current = snapshot.value
  if (!current) return null
  if (store.preflight?.observedPort === portDraft.value) return store.preflight
  return current.port === portDraft.value ? current.portObservation : null
})

const portCanApply = computed(() => {
  const current = snapshot.value
  if (!current || portDraft.value === current.port) return false
  if (current.mode === 'named' && portDraft.value !== 8767) return false
  return (
    allowed('updatePort') &&
    actionablePortObservation(activePortObservation.value, portDraft.value, current.port)
  )
})

const configureValid = computed(() => {
  const current = snapshot.value
  if (!current || !allowed('configureTunnel')) return false
  if (!['none', 'quick', 'named'].includes(modeDraft.value)) return false
  if (modeDraft.value !== 'named') return true
  const tokenOK = current.tunnel.token_state === 'stored' || validTokenInput(tunnelToken.value)
  return validNamedOrigin(namedOrigin.value) && tokenOK
})

const configureChanged = computed(() => {
  const current = snapshot.value
  if (!current) return false
  if (modeDraft.value !== current.mode) return true
  if (modeDraft.value === 'named' && namedOrigin.value !== currentOrigin.value) return true
  return tunnelToken.value.trim() !== ''
})

const autostartKnown = computed(
  () => snapshot.value?.tunnel.autostart === 'enabled' || snapshot.value?.tunnel.autostart === 'disabled',
)

const passwordStateLabel = computed(() =>
  t(passwordStateKeys[snapshot.value?.oauthPasswordState ?? ''] ?? 'connection.password_unavailable'),
)
const endpointStateLabel = computed(() =>
  t(endpointKeys[endpointState.value] ?? 'common.unknown'),
)
const portStateLabel = computed(() =>
  t(portStateKeys[activePortObservation.value?.state ?? ''] ?? 'common.unknown'),
)
const portReasonLabel = computed(() => {
  const reason = activePortObservation.value?.reasonCode
  return reason ? t(reasonKeys[reason] ?? 'connection.reason_unavailable') : ''
})
const modeLabel = computed(() => t(modeKeys[snapshot.value?.mode ?? ''] ?? 'common.unknown'))
const confirmationPrompt = computed(() => {
  const pending = confirmation.value
  if (!pending) return ''
  if (pending.kind === 'action') {
    return t('connection.confirm_action', { action: t(actionKeys[pending.action]) })
  }
  if (pending.kind === 'port') return t('connection.confirm_port', { port: pending.port })
  if (pending.kind === 'autostart') {
    return t('connection.confirm_autostart', {
      state: t(pending.enabled ? 'common.enabled' : 'common.disabled'),
    })
  }
  return t('connection.confirm_tunnel', { mode: t(modeKeys[pending.mode] ?? 'common.unknown') })
})

function allowed(name: string) {
  return store.canPerform(name) && contract.canInvoke(Domain.DomainConnection, name)
}

function disabledReason(name: string) {
  if (!contract.canInvoke(Domain.DomainConnection, name)) return t('connection.reason_unavailable')
  const reason = store.disabledReason(name)
  return reason ? t(reasonKeys[reason] ?? 'connection.reason_unavailable') : ''
}

function triState(value: boolean | null) {
  if (value === true) return t('common.yes')
  if (value === false) return t('common.no')
  return t('common.unknown')
}

function clearPassword() {
  if (secretTimer !== undefined) window.clearTimeout(secretTimer)
  secretTimer = undefined
  oauthPassword.value = ''
  passwordVisible.value = false
}

function clearCopyNotice() {
  if (copyTimer !== undefined) window.clearTimeout(copyTimer)
  copyTimer = undefined
  copyNotice.value = null
  copyFailed.value = false
}

function scheduleSecretClear() {
  if (secretTimer !== undefined) window.clearTimeout(secretTimer)
  secretTimer = window.setTimeout(clearPassword, 30_000)
}

async function showPassword() {
  if (passwordVisible.value) {
    clearPassword()
    return
  }
  const result = await store.revealOAuthPassword()
  if (
    result?.state !== OAuthPasswordState.OAuthPasswordStored ||
    !result.password ||
    result.configRevision !== snapshot.value?.configRevision
  ) {
    clearPassword()
    return
  }
  oauthPassword.value = result.password
  passwordVisible.value = true
  scheduleSecretClear()
}

async function copyText(value: string, kind: 'local' | 'public' | 'password') {
  clearCopyNotice()
  try {
    await navigator.clipboard.writeText(value)
    copyNotice.value = kind
    copyTimer = window.setTimeout(clearCopyNotice, 2_500)
  } catch {
    copyFailed.value = true
    copyTimer = window.setTimeout(clearCopyNotice, 2_500)
  }
}

async function copyPassword() {
  let value = passwordVisible.value ? oauthPassword.value : ''
  if (!value) {
    const result = await store.revealOAuthPassword()
    if (
      result?.state !== OAuthPasswordState.OAuthPasswordStored ||
      !result.password ||
      result.configRevision !== snapshot.value?.configRevision
    ) {
      return
    }
    value = result.password
  }
  await copyText(value, 'password')
  value = ''
}

function validTokenInput(value: string) {
  const token = value.trim()
  return token.length > 0 && token.length <= 16 * 1024 && !/[\r\n\u0000]/.test(token)
}

function requestAction(action: ConnectionActionName) {
  if (!allowed(action)) return
  if (store.capability(action)?.requiresConfirmation ?? true) {
    confirmation.value = { kind: 'action', action }
  } else {
    void store.perform(action)
  }
}

function requestPortUpdate() {
  if (portCanApply.value) confirmation.value = { kind: 'port', port: portDraft.value }
}

function requestTunnelConfigure() {
  if (!configureValid.value || !configureChanged.value) return
  confirmation.value = {
    kind: 'tunnel',
    mode: modeDraft.value,
    origin: modeDraft.value === 'named' ? namedOrigin.value.trim() : '',
  }
}

function requestAutostart() {
  if (!allowed('setTunnelAutostart') || !autostartKnown.value) return
  confirmation.value = { kind: 'autostart', enabled: autostartDraft.value }
}

function cancelConfirmation() {
  if (confirmation.value?.kind === 'tunnel') tunnelToken.value = ''
  confirmation.value = null
}

async function confirmOperation() {
  const pending = confirmation.value
  confirmation.value = null
  if (!pending) return

  if (pending.kind === 'action') {
    await store.perform(pending.action)
    return
  }
  if (pending.kind === 'port') {
    await store.updatePort(pending.port)
    return
  }
  if (pending.kind === 'autostart') {
    await store.setTunnelAutostart(pending.enabled)
    return
  }

  const token = tunnelToken.value.trim()
  try {
    await store.configureTunnel(pending.mode, pending.origin, token || undefined)
  } finally {
    tunnelToken.value = ''
  }
}

function syncDrafts() {
  const current = snapshot.value
  if (!current) return
  portDraft.value = current.port
  modeDraft.value = current.mode
  namedOrigin.value = current.mode === 'named' ? currentOrigin.value : ''
  autostartDraft.value = current.tunnel.autostart === 'enabled'
  tunnelToken.value = ''
  store.clearPreflight()
}

watch(
  () => snapshot.value?.configRevision,
  (revision) => {
    if (!revision || revision === lastRevision) return
    lastRevision = revision
    clearPassword()
    syncDrafts()
  },
)

watch(portDraft, (candidate) => {
  if (portTimer !== undefined) window.clearTimeout(portTimer)
  store.clearPreflight()
  if (!snapshot.value || !Number.isInteger(candidate) || candidate < 1 || candidate > 65535) return
  portTimer = window.setTimeout(() => {
    void store.preflightPort(candidate)
  }, 350)
})

watch(modeDraft, (mode) => {
  if (mode !== 'named') tunnelToken.value = ''
})

function handleBlur() {
  clearPassword()
}

onMounted(() => {
  window.addEventListener('blur', handleBlur)
  void contract.load()
  void store.refresh()
})

onBeforeUnmount(() => {
  window.removeEventListener('blur', handleBlur)
  if (portTimer !== undefined) window.clearTimeout(portTimer)
  tunnelToken.value = ''
  clearPassword()
  clearCopyNotice()
})
</script>

<template>
  <section class="panel connection-panel" aria-labelledby="connection-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ t('connection.eyebrow') }}</p>
        <h2 id="connection-title">{{ t('connection.title') }}</h2>
      </div>
      <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
    </div>

    <template v-if="snapshot">
      <section class="connection-section" aria-labelledby="connection-summary-title">
        <div class="section-heading">
          <div>
            <h3 id="connection-summary-title">{{ t('connection.summary') }}</h3>
            <p class="hint">{{ t('connection.summary_hint') }}</p>
          </div>
        </div>
        <dl class="status-grid connection-readiness-grid">
          <div>
            <dt>{{ t('connection.local_core') }}</dt>
            <dd>{{ snapshot.coreRunning === null ? t('common.unknown') : t(snapshot.coreRunning ? 'common.running' : 'common.stopped') }}</dd>
          </div>
          <div>
            <dt>{{ t('connection.port_health') }}</dt>
            <dd>{{ t(portStateKeys[snapshot.portObservation.state] ?? 'common.unknown') }}</dd>
          </div>
          <div>
            <dt>{{ t('connection.public_access') }}</dt>
            <dd>{{ modeLabel }}</dd>
          </div>
          <div>
            <dt>{{ t('connection.connector_readiness') }}</dt>
            <dd>{{ t(readiness.public ? 'connection.connector_ready' : 'connection.connector_not_ready') }}</dd>
          </div>
        </dl>
        <p class="hint">
          {{ t(readiness.local ? 'connection.local_ready' : 'connection.local_not_ready') }}
          ·
          {{ t(readiness.public ? 'connection.public_ready' : 'connection.public_not_ready') }}
        </p>
      </section>

      <section class="connection-section" aria-labelledby="connector-title">
        <div class="section-heading">
          <div>
            <h3 id="connector-title">{{ t('connection.connector_title') }}</h3>
            <p class="hint">{{ t('connection.connector_hint') }}</p>
          </div>
        </div>

        <div class="connection-value-list">
          <div class="connection-value-row">
            <div>
              <span class="connection-label">{{ t('connection.local_mcp_url') }}</span>
              <code class="path safe-url">{{ localMCPURL || t('common.unavailable') }}</code>
            </div>
            <button type="button" :disabled="!localMCPURL" @click="copyText(localMCPURL, 'local')">
              {{ t('connection.copy') }}
            </button>
          </div>

          <div class="connection-value-row">
            <div>
              <span class="connection-label">{{ t('connection.public_mcp_url') }}</span>
              <code class="path safe-url">{{ publicMCPURL || t('common.unavailable') }}</code>
            </div>
            <button type="button" :disabled="!publicMCPURL" @click="copyText(publicMCPURL, 'public')">
              {{ t('connection.copy') }}
            </button>
          </div>

          <div class="connection-value-row">
            <div>
              <span class="connection-label">{{ t('connection.oauth_password') }}</span>
              <span class="secret-value">
                {{ passwordVisible && oauthPassword ? oauthPassword : '••••••••••••' }}
              </span>
              <small class="hint">{{ passwordStateLabel }}</small>
            </div>
            <div class="actions">
              <button type="button"
                :disabled="snapshot.oauthPasswordState !== OAuthPasswordState.OAuthPasswordStored || store.revealBusy || !allowed('revealOAuthPassword')"
                :aria-pressed="passwordVisible" @click="showPassword">
                {{ t(passwordVisible ? 'connection.hide_password' : 'connection.show_password') }}
              </button>
              <button type="button"
                :disabled="snapshot.oauthPasswordState !== OAuthPasswordState.OAuthPasswordStored || store.revealBusy || !allowed('revealOAuthPassword')"
                @click="copyPassword">
                {{ t('connection.copy_password') }}
              </button>
            </div>
          </div>
        </div>

        <p class="hint">{{ t('connection.password_hint') }}</p>

        <div class="connection-test-row">
          <div>
            <span class="connection-label">{{ t('connection.public_endpoint') }}</span>
            <span class="state-pill" :data-state="endpointState">{{ endpointStateLabel }}</span>
          </div>
          <button type="button"
            :disabled="!publicMCPURL || store.endpointBusy || !allowed('testPublicEndpoint')"
            @click="store.testPublicEndpoint">
            {{ store.endpointBusy ? t('connection.endpoint_checking') : t('connection.test_endpoint') }}
          </button>
        </div>

        <p class="hint" aria-live="polite">
          <template v-if="copyNotice">{{ t('connection.copied') }}</template>
          <template v-else-if="copyFailed">{{ t('connection.copy_failed') }}</template>
        </p>
      </section>

      <section class="connection-section" aria-labelledby="port-title">
        <div class="section-heading">
          <div>
            <h3 id="port-title">{{ t('connection.port_title') }}</h3>
            <p class="hint">{{ t('connection.port_hint') }}</p>
          </div>
        </div>
        <div class="connection-form-row">
          <label for="connection-port">{{ t('connection.port_label') }}</label>
          <input id="connection-port" v-model.number="portDraft" type="number" min="1" max="65535" step="1"
            :disabled="store.busy || !!confirmation" aria-describedby="connection-port-state" />
          <span id="connection-port-state" class="state-pill" :data-state="activePortObservation?.state ?? 'unknown'">
            {{ store.preflightBusy ? t('connection.port_checking') : portStateLabel }}
          </span>
          <button type="button" :disabled="!portCanApply || store.preflightBusy" @click="requestPortUpdate">
            {{ t('connection.apply_port') }}
          </button>
        </div>
        <p v-if="portReasonLabel" class="hint">{{ portReasonLabel }}</p>
        <p v-if="snapshot.mode === 'named' && portDraft !== 8767" class="hint error">
          {{ t('connection.named_port_blocked') }}
        </p>
      </section>

      <section class="connection-section" aria-labelledby="public-access-title">
        <div class="section-heading">
          <div>
            <h3 id="public-access-title">{{ t('connection.public_access_title') }}</h3>
            <p class="hint">{{ t('connection.public_access_hint') }}</p>
          </div>
        </div>

        <fieldset class="connection-mode-fieldset" :disabled="store.busy || !!confirmation">
          <legend>{{ t('connection.mode') }}</legend>
          <label v-for="mode in ['none', 'quick', 'named']" :key="mode" class="connection-mode-option">
            <input v-model="modeDraft" type="radio" name="connection-mode" :value="mode" />
            <span>
              <strong>{{ t(modeKeys[mode]) }}</strong>
              <small class="hint">{{ t(modeHelpKeys[mode]) }}</small>
            </span>
          </label>
        </fieldset>

        <div v-if="modeDraft === 'named'" class="connection-named-fields">
          <label for="named-origin">{{ t('connection.named_origin') }}</label>
          <input id="named-origin" v-model.trim="namedOrigin" type="url"
            :placeholder="t('connection.named_origin_placeholder')" autocomplete="url" />
          <p class="hint">{{ t('connection.named_origin_hint') }}</p>

          <label for="tunnel-token">{{ t('connection.tunnel_token') }}</label>
          <input id="tunnel-token" v-model="tunnelToken" type="password"
            :placeholder="t('connection.tunnel_token_placeholder')" autocomplete="new-password" />
          <p class="hint">
            {{ t(snapshot.tunnel.token_state === 'stored' ? 'connection.token_stored' : 'connection.token_missing') }}
            {{ t('connection.token_write_only') }}
          </p>
        </div>

        <p v-if="modeDraft === 'quick'" class="hint">{{ t('connection.quick_url_warning') }}</p>
        <p v-if="modeDraft === 'named'" class="hint">{{ t('connection.named_route_manual') }}</p>

        <div class="actions">
          <button type="button" :disabled="!configureValid || !configureChanged" @click="requestTunnelConfigure">
            {{ t('connection.configure') }}
          </button>
          <button v-for="action in actions" :key="action" type="button"
            :disabled="!allowed(action)" @click="requestAction(action)">
            {{ t(actionKeys[action]) }}
          </button>
        </div>
        <div class="connection-disabled-reasons" aria-live="polite">
          <p v-for="action in actions.filter((item) => !allowed(item))" :key="action" class="hint">
            {{ t(actionKeys[action]) }}: {{ disabledReason(action) }}
          </p>
        </div>
      </section>

      <details class="connection-advanced">
        <summary>{{ t('connection.advanced') }}</summary>
        <div class="connection-advanced-content">
          <dl class="status-grid">
            <div><dt>{{ t('connection.tunnel_registered') }}</dt><dd>{{ triState(snapshot.tunnel.registered) }}</dd></div>
            <div><dt>{{ t('connection.tunnel_running') }}</dt><dd>{{ triState(snapshot.tunnel.running) }}</dd></div>
            <div><dt>{{ t('connection.tunnel_connected') }}</dt><dd>{{ triState(snapshot.tunnel.connected) }}</dd></div>
            <div><dt>{{ t('connection.core_health') }}</dt><dd>{{ snapshot.coreHealth === 'healthy' ? t('common.healthy') : snapshot.coreHealth === 'unhealthy' ? t('common.not_healthy') : t('common.unknown') }}</dd></div>
          </dl>

          <div class="connection-form-row">
            <label class="checkbox-label" for="tunnel-autostart">
              <input id="tunnel-autostart" v-model="autostartDraft" type="checkbox"
                :disabled="!autostartKnown || !allowed('setTunnelAutostart') || store.busy" />
              {{ t('connection.tunnel_autostart') }}
            </label>
            <button type="button"
              :disabled="!autostartKnown || !allowed('setTunnelAutostart') || autostartDraft === (snapshot.tunnel.autostart === 'enabled')"
              @click="requestAutostart">
              {{ t('common.save') }}
            </button>
          </div>
          <p v-if="!allowed('setTunnelAutostart')" class="hint">{{ disabledReason('setTunnelAutostart') }}</p>

          <p v-if="snapshot.tunnel.remote_route === 'manual_route_required'" class="hint">
            {{ t('connection.named_route_manual') }}
          </p>
          <p v-if="snapshot.tunnel.recovery_required" class="error">
            {{ t('connection.recovery_required') }}
          </p>
          <p class="hint">{{ t('connection.generation', { generation: snapshot.tunnelGeneration || t('common.unavailable') }) }}</p>
        </div>
      </details>

      <p v-if="store.phase === 'waiting_readiness'" class="hint" aria-live="polite">
        {{ t('connection.waiting_readiness') }}
      </p>
    </template>

    <p v-else-if="!store.busy">{{ t('connection.state_unavailable') }}</p>

    <OperationStatus :busy="store.busy" :error="store.error" :completed="store.completed" />

    <ConfirmDialog v-if="confirmation" :prompt="confirmationPrompt"
      @confirm="confirmOperation" @cancel="cancelConfirmation">
      <dl v-if="confirmation.kind === 'port'" class="confirmation-values">
        <dt>{{ t('connection.port_label') }}</dt><dd>{{ confirmation.port }}</dd>
      </dl>
      <dl v-else-if="confirmation.kind === 'tunnel'" class="confirmation-values">
        <dt>{{ t('connection.mode') }}</dt><dd>{{ t(modeKeys[confirmation.mode] ?? 'common.unknown') }}</dd>
        <template v-if="confirmation.mode === 'named'">
          <dt>{{ t('connection.named_origin') }}</dt><dd class="safe-url">{{ confirmation.origin }}</dd>
        </template>
      </dl>
      <dl v-else-if="confirmation.kind === 'autostart'" class="confirmation-values">
        <dt>{{ t('connection.tunnel_autostart') }}</dt>
        <dd>{{ t(confirmation.enabled ? 'common.enabled' : 'common.disabled') }}</dd>
      </dl>
      <p v-if="confirmation.kind === 'action' && confirmation.action === 'regenerate'" class="hint">
        {{ t('connection.quick_url_warning') }}
      </p>
      <p v-if="confirmation.kind === 'tunnel' && confirmation.mode === 'named'" class="hint">
        {{ t('connection.confirm_token_omitted') }}
      </p>
    </ConfirmDialog>
  </section>
</template>
