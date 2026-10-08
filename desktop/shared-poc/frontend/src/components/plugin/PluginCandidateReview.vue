<script setup lang="ts">
import { computed, onMounted, useTemplateRef } from 'vue'
import type { PluginCandidate } from '../../api/desktopApi'
import { candidateDiff, candidateSummary, shortFingerprint } from '../../features/pluginLogic'
import { useI18n } from '../../i18n'

const props = defineProps<{ candidate: PluginCandidate; sourceLabel: string; busy: boolean; currentlyEnabled?: boolean }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const { t } = useI18n()
const summary = computed(() => candidateSummary(props.candidate.review))
const update = computed(() => props.candidate.kind === 'update')
const diff = computed(() => candidateDiff(props.candidate.current, props.candidate.review))
const hasDiff = computed(() => {
  const value = diff.value
  if (!value) return false
  return value.fingerprintChanged ||
    value.skills.added.length > 0 || value.skills.removed.length > 0 || value.skills.changed.length > 0 ||
    value.mcp.added.length > 0 || value.mcp.removed.length > 0 || value.mcp.changed.length > 0 ||
    value.executables.added.length > 0 || value.executables.removed.length > 0 ||
    value.warnings.added.length > 0 || value.warnings.resolved.length > 0
})
onMounted(() => dialog.value?.showModal())
</script>

<template>
  <dialog ref="dialog" class="plugin-review-dialog" aria-labelledby="plugin-review-title" @cancel.prevent="emit('cancel')">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ update ? t('plugin.review_update_eyebrow') : t('plugin.review_install_eyebrow') }}</p>
        <h2 id="plugin-review-title">{{ t('plugin.review_title') }}</h2>
        <p class="execution-meta">{{ sourceLabel || t('plugin.review_private_source') }}</p>
      </div>
      <span class="state-pill" :data-state="candidate.review.valid ? 'healthy' : 'unavailable'">
        {{ candidate.review.valid ? t('plugin.review_valid') : t('plugin.review_invalid') }}
      </span>
    </div>

    <div v-if="update && candidate.current" class="plugin-version-diff">
      <div><span>{{ t('plugin.current_version') }}</span><strong>{{ candidate.current.version }}</strong></div>
      <span aria-hidden="true">→</span>
      <div><span>{{ t('plugin.candidate_version') }}</span><strong>{{ candidate.review.version }}</strong></div>
    </div>

    <dl class="status-grid plugin-review-grid">
      <div><dt>{{ t('plugin.name') }}</dt><dd>{{ candidate.review.name }}</dd></div>
      <div><dt>{{ t('plugin.version') }}</dt><dd>{{ candidate.review.version }}</dd></div>
      <div><dt>{{ t('plugin.format') }}</dt><dd>{{ candidate.review.format }}</dd></div>
      <div><dt>{{ t('plugin.fingerprint') }}</dt><dd class="mono">{{ shortFingerprint(candidate.review.packageFingerprint) }}</dd></div>
      <div><dt>{{ t('plugin.skills') }}</dt><dd>{{ summary.skills }}</dd></div>
      <div><dt>{{ t('plugin.mcp_components') }}</dt><dd>{{ summary.mcp }}</dd></div>
      <div><dt>{{ t('plugin.executables') }}</dt><dd>{{ summary.executables }}</dd></div>
      <div><dt>{{ t('plugin.warnings') }}</dt><dd>{{ summary.warnings }}</dd></div>
    </dl>

    <p v-if="candidate.review.description" class="plugin-review-description">{{ candidate.review.description }}</p>
    <p v-if="candidate.review.provenance" class="hint">
      {{ t('plugin.provenance') }}:
      {{ candidate.review.provenance.origin || t('common.unavailable') }}
      <span v-if="candidate.review.provenance.ref"> · {{ candidate.review.provenance.ref }}</span>
      <span v-if="candidate.review.provenance.revision"> · {{ candidate.review.provenance.revision }}</span>
      <span v-if="candidate.review.provenance.subdir"> · {{ candidate.review.provenance.subdir }}</span>
    </p>

    <section v-if="candidate.review.skills?.length" class="plugin-review-section">
      <h3>{{ t('plugin.skills') }}</h3>
      <article v-for="skill in candidate.review.skills" :key="skill.name" class="plugin-component-card">
        <strong>{{ skill.name }}</strong>
        <p v-if="skill.description" class="hint">{{ skill.description }}</p>
      </article>
    </section>

    <section v-if="update && diff" class="plugin-review-section">
      <h3>{{ t('plugin.update_changes') }}</h3>
      <p v-if="!hasDiff" class="hint">{{ t('plugin.update_no_changes') }}</p>
      <div v-else class="plugin-diff-grid">
        <div>
          <h4>{{ t('plugin.skills') }}</h4>
          <p v-if="diff.skills.added.length">{{ t('plugin.diff_added') }}: {{ diff.skills.added.join(', ') }}</p>
          <p v-if="diff.skills.removed.length">{{ t('plugin.diff_removed') }}: {{ diff.skills.removed.join(', ') }}</p>
          <p v-if="diff.skills.changed.length">{{ t('plugin.diff_changed') }}: {{ diff.skills.changed.join(', ') }}</p>
        </div>
        <div>
          <h4>{{ t('plugin.mcp_components') }}</h4>
          <p v-if="diff.mcp.added.length">{{ t('plugin.diff_added') }}: {{ diff.mcp.added.join(', ') }}</p>
          <p v-if="diff.mcp.removed.length">{{ t('plugin.diff_removed') }}: {{ diff.mcp.removed.join(', ') }}</p>
          <p v-if="diff.mcp.changed.length">{{ t('plugin.diff_changed') }}: {{ diff.mcp.changed.join(', ') }}</p>
        </div>
        <div>
          <h4>{{ t('plugin.executables') }}</h4>
          <p v-if="diff.executables.added.length">{{ t('plugin.diff_added') }}: {{ diff.executables.added.join(', ') }}</p>
          <p v-if="diff.executables.removed.length">{{ t('plugin.diff_removed') }}: {{ diff.executables.removed.join(', ') }}</p>
        </div>
        <div>
          <h4>{{ t('plugin.warnings') }}</h4>
          <p v-if="diff.warnings.added.length">{{ t('plugin.diff_added') }}: {{ diff.warnings.added.join(', ') }}</p>
          <p v-if="diff.warnings.resolved.length">{{ t('plugin.diff_resolved') }}: {{ diff.warnings.resolved.join(', ') }}</p>
          <p>{{ t(diff.fingerprintChanged ? 'plugin.fingerprint_changed' : 'plugin.fingerprint_unchanged') }}</p>
        </div>
      </div>
    </section>

    <section v-if="candidate.review.mcp?.length" class="plugin-review-section">
      <h3>{{ t('plugin.mcp_components') }}</h3>
      <article v-for="component in candidate.review.mcp" :key="component.name" class="plugin-component-card">
        <div class="plugin-component-heading"><strong>{{ component.name }}</strong><span>{{ component.transport }}</span></div>
        <p v-if="component.description" class="hint">{{ component.description }}</p>
        <p v-if="component.endpoint" class="mono plugin-safe-value">{{ component.endpoint }}</p>
        <p v-if="component.command" class="mono plugin-safe-value">{{ component.command }}</p>
        <p v-if="component.environmentNames?.length" class="hint">{{ t('plugin.environment_names') }}: {{ component.environmentNames.join(', ') }}</p>
        <p v-if="component.headerNames?.length" class="hint">{{ t('plugin.header_names') }}: {{ component.headerNames.join(', ') }}</p>
      </article>
    </section>

    <section v-if="candidate.review.executables?.length || candidate.review.warnings?.length || candidate.review.issues?.length" class="plugin-review-section">
      <h3>{{ t('plugin.review_attention') }}</h3>
      <ul class="plugin-review-list">
        <li v-for="item in candidate.review.executables" :key="'exe:' + item">{{ t('plugin.executable_item', { value: item }) }}</li>
        <li v-for="item in candidate.review.warnings" :key="'warn:' + item">{{ item }}</li>
        <li v-for="item in candidate.review.issues" :key="'issue:' + item" class="error">{{ item }}</li>
      </ul>
    </section>

    <div class="restart-notice" role="status">
      <strong>{{ update ? t('plugin.update_runtime_notice_title') : t('plugin.install_disabled_title') }}</strong>
      <p v-if="!update">{{ t('plugin.install_disabled_notice') }}</p>
      <p v-else-if="currentlyEnabled">{{ t('plugin.update_runtime_notice') }}</p>
      <p v-else>{{ t('plugin.update_disabled_notice') }}</p>
    </div>

    <div class="actions plugin-review-actions">
      <button type="button" autofocus :disabled="busy" @click="emit('cancel')">{{ t('common.cancel') }}</button>
      <button type="button" :disabled="busy || !candidate.review.valid || summary.issues > 0" @click="emit('confirm')">
        {{ update ? t('plugin.confirm_update') : t('plugin.confirm_install') }}
      </button>
    </div>
  </dialog>
</template>
