<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { PluginManagedItem, PluginMCPComponent } from '../../api/desktopApi'
import { useI18n } from '../../i18n'
import { usePluginStore } from '../../stores/plugin'
import ConfirmDialog from '../common/ConfirmDialog.vue'

const props = defineProps<{
  plugin: PluginManagedItem
  component: PluginMCPComponent
}>()

const store = usePluginStore()
const { t } = useI18n()
const key = ref('')
const secret = ref('')
const unsetTarget = ref('')

const environment = computed(() => store.environmentFor(props.plugin.name, props.component.name))
const items = computed(() => environment.value?.items ?? [])

async function reload() {
  await store.readEnvironment(props.plugin, props.component.name)
}

function replace(keyName: string) {
  key.value = keyName
  secret.value = ''
}

async function saveSecret() {
  const wantedKey = key.value.trim()
  const value = secret.value
  if (!wantedKey || !value || !environment.value) return
  try {
    await store.setEnvironment(props.plugin, props.component.name, wantedKey, value)
    if (!store.mutationError) key.value = ''
  } finally {
    secret.value = ''
  }
}

async function confirmUnset() {
  const target = unsetTarget.value
  unsetTarget.value = ''
  if (!target || !environment.value) return
  await store.unsetEnvironment(props.plugin, props.component.name, target)
}

watch(() => props.plugin.generation, () => void reload())
onMounted(() => void reload())
onUnmounted(() => { secret.value = '' })
</script>

<template>
  <section class="plugin-env-section" :aria-labelledby="'plugin-env-' + component.name">
    <div class="section-heading">
      <div>
        <h4 :id="'plugin-env-' + component.name">{{ t('plugin.environment_title') }}</h4>
        <p class="field-hint">{{ t('plugin.environment_hint') }}</p>
      </div>
      <button type="button" :disabled="store.busy" @click="reload">{{ t('common.refresh') }}</button>
    </div>

    <div v-if="environment" class="mcp-env-list">
      <div v-for="item in items" :key="item.key" class="mcp-env-row">
        <div>
          <strong class="mono">{{ item.key }}</strong>
          <span class="state-pill" :data-state="item.configured ? 'healthy' : 'unavailable'">
            {{ item.configured ? t('plugin.configured') : t('plugin.missing') }}
          </span>
        </div>
        <div class="actions">
          <button type="button" :disabled="store.busy" @click="replace(item.key)">{{ t('plugin.replace_value') }}</button>
          <button
            type="button"
            class="danger-button"
            :disabled="store.busy || !item.configured"
            @click="unsetTarget = item.key"
          >{{ t('plugin.unset_value') }}</button>
        </div>
      </div>
      <p v-if="items.length === 0" class="empty-state">{{ t('plugin.environment_empty') }}</p>
    </div>
    <p v-else class="hint">{{ t('common.loading') }}</p>

    <form class="mcp-secret-form" @submit.prevent="saveSecret">
      <label>
        {{ t('plugin.environment_key') }}
        <select v-model="key" :disabled="store.busy">
          <option value="">{{ t('plugin.choose_environment_key') }}</option>
          <option v-for="item in items" :key="item.key" :value="item.key">{{ item.key }}</option>
        </select>
      </label>
      <label>
        {{ t('plugin.environment_value') }}
        <input v-model="secret" type="password" autocomplete="new-password" :disabled="store.busy" />
      </label>
      <p class="field-hint">{{ t('plugin.environment_write_only') }}</p>
      <button type="submit" :disabled="store.busy || !key || !secret">{{ t('plugin.save_value') }}</button>
    </form>

    <ConfirmDialog
      v-if="unsetTarget"
      :prompt="t('plugin.confirm_unset', { key: unsetTarget })"
      @cancel="unsetTarget = ''"
      @confirm="confirmUnset"
    >
      <p>{{ t('plugin.confirm_unset_hint') }}</p>
    </ConfirmDialog>
  </section>
</template>
