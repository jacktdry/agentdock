<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import type { MCPConfigInput, MCPManagedServer } from '../../api/desktopApi'
import { useI18n } from '../../i18n'
import { useContractStore } from '../../stores/contract'
import { useMCPStore } from '../../stores/mcp'
import MCPServerDetail from './MCPServerDetail.vue'
import MCPServerEditor from './MCPServerEditor.vue'
import MCPServerList from './MCPServerList.vue'

const store = useMCPStore()
const contract = useContractStore()
const { t } = useI18n()

const selectedName = ref('')
const editorOpen = ref(false)
const editorServer = shallowRef<MCPManagedServer | null>(null)
const editorRevision = ref('')

const standalone = computed(() => store.servers.filter((server) => server.sourceType !== 'plugin'))
const pluginProvided = computed(() => store.servers.filter((server) => server.sourceType === 'plugin'))
const selectedServer = computed(() => store.servers.find((server) => server.name === selectedName.value) ?? null)

async function selectServer(name: string) {
  selectedName.value = name
  store.clearTransient()
  const server = store.servers.find((item) => item.name === name)
  if (server) await store.inspect(server)
}

function openAdd() {
  editorServer.value = null
  editorRevision.value = store.snapshot?.registryRevision ?? ''
  editorOpen.value = true
}

function openEdit(server: MCPManagedServer) {
  editorServer.value = server
  editorRevision.value = store.snapshot?.registryRevision ?? ''
  editorOpen.value = true
}

async function saveServer(input: MCPConfigInput, revision: string, generation: string) {
  const editing = editorServer.value
  const result = editing
    ? await store.update(editing, input, revision, generation)
    : await store.create(input, revision)
  if (!result || result.error || result.outcomeUnknown) return
  editorOpen.value = false
  editorServer.value = null
  if (input.name) selectedName.value = input.name
}

async function reload() {
  await store.refresh()
}

watch(
  () => store.servers.map((server) => server.name).join('\u0000'),
  () => {
    if (selectedName.value && !store.servers.some((server) => server.name === selectedName.value)) {
      selectedName.value = ''
      store.clearTransient()
    }
  },
)

onMounted(async () => {
  await contract.load()
  await store.refresh()
})
</script>

<template>
  <section class="feature-stack mcp-panel" aria-labelledby="mcp-title" :aria-busy="store.busy">
    <div class="panel">
      <div class="panel-heading">
        <div>
          <h2 id="mcp-title">{{ t('mcp.title') }}</h2>
          <p class="execution-meta">{{ t('mcp.intro') }}</p>
        </div>
        <div class="actions">
          <button type="button" :disabled="store.busy || !store.canRead" @click="reload">{{ t('mcp.reload_status') }}</button>
          <button type="button" :disabled="store.busy || store.stale || !store.canInvoke('create')" @click="openAdd">{{ t('mcp.add_server') }}</button>
        </div>
      </div>

      <div v-if="store.loading && !store.snapshot" class="empty-state" role="status">{{ t('mcp.loading') }}</div>
      <div v-if="store.stale" class="restart-notice" role="alert">
        <strong>{{ t('mcp.stale_title') }}</strong>
        <p>{{ t('mcp.stale_hint') }}</p>
      </div>
      <div v-else-if="store.readError" class="error" role="alert">{{ t('mcp.read_error') }}</div>

      <dl v-if="store.snapshot" class="status-grid mcp-summary-grid">
        <div><dt>{{ t('mcp.standalone_title') }}</dt><dd>{{ t('mcp.summary_standalone', { count: standalone.length }) }}</dd></div>
        <div><dt>{{ t('mcp.plugin_title') }}</dt><dd>{{ t('mcp.summary_plugin', { count: pluginProvided.length }) }}</dd></div>
        <div><dt>{{ t('mcp.status_available') }}</dt><dd>{{ t('mcp.summary_available', { count: store.availableCount }) }}</dd></div>
        <div><dt>{{ t('mcp.status_needs_attention') }}</dt><dd>{{ t('mcp.summary_attention', { count: store.attentionCount }) }}</dd></div>
      </dl>

      <div v-if="store.lastFeedback?.outcomeUnknown" class="restart-notice" role="status">{{ t('mcp.outcome_unknown') }}</div>
      <div v-else-if="store.lastFeedback?.recoveryRequired" class="restart-notice" role="status">{{ t('mcp.partial_title') }}</div>
      <div v-if="store.lastFeedback?.reconnectRequired" class="restart-notice" role="status">{{ t('mcp.reconnect_required') }}</div>
      <p v-if="store.mutationError" class="error" role="alert">{{ t('mcp.mutation_error') }} <span class="mono">{{ store.mutationError.code }}</span></p>
    </div>

    <div v-if="store.snapshot" class="mcp-workspace" :class="{ 'has-selection': !!selectedServer }">
      <div class="mcp-browser">
        <section class="panel" aria-labelledby="mcp-standalone-title">
          <div class="panel-heading">
            <div>
              <h3 id="mcp-standalone-title">{{ t('mcp.standalone_title') }}</h3>
              <p class="execution-meta">{{ t('mcp.standalone_hint') }}</p>
            </div>
            <button type="button" :disabled="store.busy || store.stale || !store.canInvoke('create')" @click="openAdd">{{ t('mcp.add_server') }}</button>
          </div>
          <MCPServerList
            :servers="standalone"
            :selected-name="selectedName"
            empty-key="mcp.empty_standalone"
            @select="selectServer"
          />
        </section>

        <details class="panel mcp-plugin-section">
          <summary>{{ t('mcp.plugin_title') }} · {{ pluginProvided.length }}</summary>
          <p class="execution-meta">{{ t('mcp.plugin_hint') }}</p>
          <MCPServerList
            :servers="pluginProvided"
            :selected-name="selectedName"
            empty-key="mcp.empty_plugin"
            @select="selectServer"
          />
        </details>
      </div>

      <MCPServerDetail
        v-if="selectedServer"
        :key="selectedServer.name + ':' + selectedServer.generation"
        :server="selectedServer"
        :registry-revision="store.snapshot.registryRevision"
        @edit="openEdit(selectedServer)"
        @removed="selectedName = ''"
        @back="selectedName = ''"
      />
      <div v-else class="panel mcp-detail-empty">
        <h3>{{ t('mcp.detail_title') }}</h3>
        <p class="hint">{{ t('mcp.select_server') }}</p>
      </div>
    </div>

    <MCPServerEditor
      v-if="editorOpen"
      :server="editorServer"
      :registry-revision="editorRevision"
      :busy="store.busy"
      @save="saveServer"
      @cancel="editorOpen = false"
    />
  </section>
</template>
