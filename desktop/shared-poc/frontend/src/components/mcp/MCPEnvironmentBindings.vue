<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { MCPManagedServer } from '../../api/desktopApi'
import { useMCPStore } from '../../stores/mcp'
import { useI18n } from '../../i18n'
import ConfirmDialog from '../common/ConfirmDialog.vue'

const props = defineProps<{
  server: MCPManagedServer
  registryRevision: string
}>()
const store = useMCPStore()
const { t } = useI18n()

const key = ref('')
const secret = ref('')
const unsetTarget = ref('')
const purgeOpen = ref(false)
const environment = computed(() => store.environmentFor(props.server.name))
const environmentItems = computed(() => environment.value?.items ?? [])

async function reload() {
  await store.readEnvironment(props.server, props.registryRevision, props.server.generation)
}

async function saveSecret() {
  const wantedKey = key.value.trim()
  const value = secret.value
  if (!wantedKey || !value || !environment.value) return
  try {
    await store.setEnvironment(
      props.server,
      wantedKey,
      value,
      props.registryRevision,
      props.server.generation,
      environment.value.revision ?? '',
    )
    if (!store.mutationError) key.value = ''
  } finally {
    secret.value = ''
  }
}

async function confirmUnset() {
  const target = unsetTarget.value
  unsetTarget.value = ''
  if (!target || !environment.value) return
  await store.unsetEnvironment(
    props.server,
    target,
    props.registryRevision,
    props.server.generation,
    environment.value.revision ?? '',
  )
}

async function confirmPurge() {
  purgeOpen.value = false
  if (!environment.value) return
  await store.purgeEnvironment(
    props.server,
    props.registryRevision,
    props.server.generation,
    environment.value.revision ?? '',
  )
}

function replace(keyName: string) {
  key.value = keyName
  secret.value = ''
}

watch(() => props.server.generation, () => void reload())
onMounted(() => void reload())
onUnmounted(() => { secret.value = '' })
</script>

<template>
  <section class="mcp-subsection" aria-labelledby="mcp-env-title">
    <div class="section-heading">
      <h3 id="mcp-env-title">{{ t('mcp.environment_title') }}</h3>
      <p class="field-hint">{{ t('mcp.environment_hint') }}</p>
    </div>

    <div v-if="environment" class="mcp-env-list">
      <div v-for="item in environmentItems" :key="item.key" class="mcp-env-row">
        <div>
          <strong class="mono">{{ item.key }}</strong>
          <span class="state-pill" :data-state="item.configured ? 'healthy' : 'unavailable'">
            {{ item.configured ? t('mcp.configured') : t('mcp.missing') }}
          </span>
        </div>
        <div class="actions">
          <button type="button" :disabled="store.busy" @click="replace(item.key)">{{ t('mcp.replace_value') }}</button>
          <button type="button" class="danger-button" :disabled="store.busy" @click="unsetTarget = item.key">{{ t('mcp.unset') }}</button>
        </div>
      </div>
      <p v-if="environmentItems.length === 0" class="empty-state">{{ t('mcp.environment_empty') }}</p>
    </div>
    <p v-else class="hint">{{ t('common.loading') }}</p>

    <form class="mcp-secret-form" @submit.prevent="saveSecret">
      <label>
        {{ t('mcp.environment_key') }}
        <input v-model="key" type="text" autocomplete="off" :disabled="store.busy" />
      </label>
      <label>
        {{ t('mcp.environment_value') }}
        <input v-model="secret" type="password" autocomplete="new-password" :disabled="store.busy" />
      </label>
      <p class="field-hint">{{ t('mcp.environment_write_only') }}</p>
      <button type="submit" :disabled="store.busy || !key.trim() || !secret">{{ t('mcp.save_value') }}</button>
    </form>

    <div class="danger-zone">
      <p class="field-hint">{{ t('mcp.remove_retains_environment') }}</p>
      <button type="button" class="danger-button" :disabled="store.busy || environmentItems.length === 0" @click="purgeOpen = true">
        {{ t('mcp.purge_environment') }}
      </button>
    </div>

    <ConfirmDialog v-if="unsetTarget" :prompt="t('mcp.confirm_unset', { key: unsetTarget })" @cancel="unsetTarget = ''" @confirm="confirmUnset" />
    <ConfirmDialog v-if="purgeOpen" :prompt="t('mcp.confirm_purge_environment')" @cancel="purgeOpen = false" @confirm="confirmPurge">
      <p class="error">{{ t('mcp.purge_environment_warning') }}</p>
    </ConfirmDialog>
  </section>
</template>
