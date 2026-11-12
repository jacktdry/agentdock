<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef, watch } from 'vue'
import { desktopApi, type NexusMutationResult, type NexusSnapshotResult } from '../../api/desktopApi'
import { nexusErrorKey, nexusOrigin } from '../../features/nexusLogic'
import { useI18n } from '../../i18n'
import ConfirmDialog from '../common/ConfirmDialog.vue'
const props = defineProps<{ snapshot: NexusSnapshotResult | null; enabled: boolean; busy: boolean }>()
const emit = defineEmits<{ submitted: [pending: Promise<NexusMutationResult>] }>()
const { t } = useI18n()
const endpoint = shallowRef('')
const name = shallowRef('')
const code = shallowRef('') // The only retained secret state; never emitted or stored.
const confirming = shallowRef(false)
const approvedGeneration = shallowRef('')
const targetOrigin = computed(() => nexusOrigin(endpoint.value.trim()))
const beforeOrigin = computed(() => nexusOrigin(props.snapshot?.safeOrigin ?? ''))
const replacing = computed(() => props.snapshot?.pairingState === 'paired')
const authorized = computed(() => !replacing.value || approvedGeneration.value === props.snapshot?.generation)
let codeTimer: ReturnType<typeof setTimeout> | undefined
function clearCode() { clearTimeout(codeTimer); code.value = '' }
function cancel() { clearCode(); confirming.value = false; approvedGeneration.value = '' }
function approve() {
  confirming.value = false
  if (props.enabled && props.snapshot?.generation && targetOrigin.value) approvedGeneration.value = props.snapshot.generation
}
function submit() {
  const current = props.snapshot
  if (!props.enabled || props.busy || !current?.generation || !targetOrigin.value || !authorized.value || !code.value.trim()) {
    clearCode()
    return
  }
  // Consume once. Clear local state before transport and never replay after errors.
  const pending = desktopApi.nexusPair({ endpoint: endpoint.value.trim(), name: name.value.trim(), code: code.value,
    expectedGeneration: current.generation, confirmReplace: replacing.value })
  clearCode()
  approvedGeneration.value = ''
  emit('submitted', pending)
}
watch(code, (value) => {
  clearTimeout(codeTimer)
  if (value) codeTimer = setTimeout(clearCode, 30_000)
})
watch([endpoint, () => props.snapshot?.generation], () => { cancel() })
watch(() => props.enabled, (enabled) => { if (!enabled) clearCode() })
onMounted(() => window.addEventListener('blur', cancel))
onBeforeUnmount(() => { cancel(); window.removeEventListener('blur', cancel) })
</script>
<template>
  <section class="connection-section">
    <h3>{{ t(replacing ? 'nexus.replace' : 'nexus.pair') }}</h3>
    <p class="hint">{{ t('nexus.code_help') }}</p>
    <form class="nexus-form" autocomplete="off" @submit.prevent="submit" @focusout="($event.relatedTarget === null) && clearCode()">
      <label>{{ t('nexus.endpoint') }}<input v-model="endpoint" type="url" required maxlength="4096" :disabled="busy || confirming" autocomplete="off" /></label>
      <label>{{ t('nexus.name') }}<input v-model="name" type="text" maxlength="256" :disabled="busy" autocomplete="off" /></label>
      <template v-if="authorized">
        <label>{{ t('nexus.code') }}<input v-model="code" type="password" required maxlength="512" autocomplete="off" autocapitalize="off" spellcheck="false" :disabled="!enabled" /></label>
        <button type="submit" :disabled="!enabled || !targetOrigin || !code.trim()">{{ t(replacing ? 'nexus.replace' : 'nexus.pair') }}</button>
      </template>
      <button v-else type="button" :disabled="!enabled || !targetOrigin" @click="confirming = true">{{ t('nexus.review_replace') }}</button>
      <button type="button" :disabled="busy" @click="cancel">{{ t('common.cancel') }}</button>
    </form>
    <p v-if="!enabled && !busy" class="hint">{{ t(nexusErrorKey(snapshot?.capabilities.pairDisabledReason)) }}</p>
    <ConfirmDialog v-if="confirming" :prompt="t('nexus.confirm_replace')" @cancel="cancel" @confirm="approve">
      <dl><dt>{{ t('nexus.before') }}</dt><dd>{{ beforeOrigin || t('nexus.unknown') }}</dd><dt>{{ t('nexus.target') }}</dt><dd>{{ targetOrigin }}</dd></dl>
      <p>{{ t('nexus.replace_warning') }}</p><p>{{ t('nexus.restart_warning') }}</p>
    </ConfirmDialog>
  </section>
</template>
<style scoped>
.nexus-form { display: grid; gap: 0.8rem; min-width: 0; }
.nexus-form label { display: grid; gap: 0.4rem; min-width: 0; }
.nexus-form input { width: 100%; min-width: 0; box-sizing: border-box; }
.nexus-form button { white-space: normal; overflow-wrap: anywhere; }
</style>
