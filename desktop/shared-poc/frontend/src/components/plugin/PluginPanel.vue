<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from '../../i18n'
import { useContractStore } from '../../stores/contract'
import { usePluginStore } from '../../stores/plugin'
import PluginCandidateReview from './PluginCandidateReview.vue'
import PluginDetail from './PluginDetail.vue'
import PluginList from './PluginList.vue'

const props = defineProps<{ focusName?: string }>()
const emit = defineEmits<{ openMcp: [] }>()
const store = usePluginStore()
const contract = useContractStore()
const { t } = useI18n()
const selectedName = ref('')

const selectedPlugin = computed(() => store.plugins.find((plugin) => plugin.name === selectedName.value) ?? null)
const recoveryItems = computed(() => store.snapshot?.recoveryItems ?? [])
const candidateTarget = computed(() => {
  const name = store.candidate?.targetName
  return name ? store.plugins.find((plugin) => plugin.name === name) ?? null : null
})
const skillProviders = computed(() => store.plugins.filter((plugin) => plugin.skillsCount > 0).length)
const mcpProviders = computed(() => store.plugins.filter((plugin) => plugin.mcpCount > 0).length)

async function selectPlugin(name: string) {
  selectedName.value = name
  store.clearTransient()
  const plugin = store.plugins.find((item) => item.name === name)
  if (plugin) await store.inspect(plugin)
}

async function install(sourceType: 'folder' | 'zip') {
  await store.chooseCandidate('install', sourceType)
}

async function confirmCandidate() {
  const candidate = store.candidate
  if (!candidate) return
  if (candidate.kind === 'install') {
    const name = candidate.review.name
    const result = await store.installCandidate()
    if (result?.completed && !result.error && !result.outcomeUnknown) selectedName.value = name
    return
  }
  if (candidateTarget.value) await store.updateCandidate(candidateTarget.value)
}

async function cancelCandidate() {
  await store.discardCandidate()
}

async function reload() {
  await store.refresh()
}

function applyFocus() {
  const name = props.focusName?.trim()
  if (name && store.plugins.some((plugin) => plugin.name === name)) void selectPlugin(name)
}

watch(() => props.focusName, applyFocus)
watch(
  () => store.plugins.map((plugin) => plugin.name + ':' + plugin.generation).join('\u0000'),
  () => {
    if (selectedName.value && !store.plugins.some((plugin) => plugin.name === selectedName.value)) {
      selectedName.value = ''
      store.clearTransient()
    }
    applyFocus()
  },
)

onMounted(async () => {
  await contract.load()
  await store.refresh()
  applyFocus()
})
</script>

<template>
  <section class="feature-stack plugin-panel" aria-labelledby="plugin-title" :aria-busy="store.busy">
    <div class="panel">
      <div class="panel-heading">
        <div>
          <h2 id="plugin-title">{{ t('plugin.title') }}</h2>
          <p class="execution-meta">{{ t('plugin.intro') }}</p>
        </div>
        <div class="actions">
          <button type="button" :disabled="store.busy || !store.canRead" @click="reload">{{ t('plugin.reload_status') }}</button>
          <button type="button" :disabled="!store.canInvoke('chooseCandidate')" @click="install('folder')">{{ t('plugin.install_folder') }}</button>
          <button type="button" :disabled="!store.canInvoke('chooseCandidate')" @click="install('zip')">{{ t('plugin.install_zip') }}</button>
        </div>
      </div>

      <div v-if="store.loading && !store.snapshot" class="empty-state" role="status">{{ t('plugin.loading') }}</div>
      <div v-if="store.stale" class="restart-notice" role="alert">
        <strong>{{ t('plugin.stale_title') }}</strong>
        <p>{{ t('plugin.stale_hint') }}</p>
      </div>
      <p v-else-if="store.readError" class="error" role="alert">{{ t('plugin.read_error') }}</p>

      <dl v-if="store.snapshot" class="status-grid plugin-summary-grid">
        <div><dt>{{ t('plugin.installed') }}</dt><dd>{{ store.plugins.length }}</dd></div>
        <div><dt>{{ t('plugin.enabled') }}</dt><dd>{{ store.enabledCount }}</dd></div>
        <div><dt>{{ t('plugin.needs_attention') }}</dt><dd>{{ store.attentionCount + recoveryItems.length }}</dd></div>
        <div><dt>{{ t('plugin.provides_skills') }}</dt><dd>{{ skillProviders }}</dd></div>
        <div><dt>{{ t('plugin.provides_mcp') }}</dt><dd>{{ mcpProviders }}</dd></div>
      </dl>

      <div v-if="store.lastFeedback?.outcomeUnknown" class="restart-notice" role="status">{{ t('plugin.outcome_unknown') }}</div>
      <div v-else-if="store.lastFeedback?.recoveryRequired" class="restart-notice" role="status">{{ t('plugin.recovery_required') }}</div>
      <div v-if="store.lastFeedback?.runtimeImpact === 'next_connection'" class="restart-notice" role="status">{{ t('plugin.reconnect_required') }}</div>
      <p v-if="store.mutationError" class="error" role="alert">
        {{ t('plugin.mutation_error') }} <span class="mono">{{ store.mutationError.code }}</span>
      </p>
    </div>

    <section v-if="recoveryItems.length" class="panel" aria-labelledby="plugin-recovery-title">
      <div class="panel-heading">
        <div>
          <h3 id="plugin-recovery-title">{{ t('plugin.cleanup_required') }}</h3>
          <p class="execution-meta">{{ t('plugin.cleanup_required_hint') }}</p>
        </div>
      </div>
      <div class="plugin-recovery-list">
        <div v-for="item in recoveryItems" :key="item.name + ':' + item.generation" class="plugin-recovery-row">
          <div>
            <strong>{{ item.name }}</strong>
            <span class="state-pill" data-state="needs_attention">{{ t('plugin.status_recovery') }}</span>
          </div>
          <button type="button" :disabled="!store.canInvoke('removePurge')" @click="store.retryPurge(item)">
            {{ t('plugin.retry_cleanup') }}
          </button>
        </div>
      </div>
    </section>

    <div v-if="store.snapshot" class="plugin-workspace" :class="{ 'has-selection': !!selectedPlugin }">
      <section class="panel plugin-browser" aria-labelledby="plugin-installed-title">
        <div class="panel-heading">
          <div>
            <h3 id="plugin-installed-title">{{ t('plugin.installed_plugins') }}</h3>
            <p class="execution-meta">{{ t('plugin.installed_hint') }}</p>
          </div>
        </div>
        <PluginList :plugins="store.plugins" :selected-name="selectedName" @select="selectPlugin" />
      </section>

      <PluginDetail
        v-if="selectedPlugin"
        :key="selectedPlugin.name + ':' + selectedPlugin.generation"
        :plugin="selectedPlugin"
        @back="selectedName = ''"
        @removed="selectedName = ''"
        @open-mcp="emit('openMcp')"
      />
      <div v-else class="panel plugin-detail-empty">
        <h3>{{ t('plugin.detail_title') }}</h3>
        <p class="hint">{{ t('plugin.select_plugin') }}</p>
      </div>
    </div>

    <PluginCandidateReview
      v-if="store.candidate"
      :candidate="store.candidate"
      :source-label="store.candidateSourceLabel"
      :busy="store.busy"
      :currently-enabled="candidateTarget?.enabled ?? false"
      @confirm="confirmCandidate"
      @cancel="cancelCandidate"
    />
  </section>
</template>
