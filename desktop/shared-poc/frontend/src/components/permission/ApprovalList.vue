<script setup lang="ts">
import type { ApprovalRecord } from '../../../bindings/github.com/uvwt/agentdock/internal/permission/models'
import { useI18n } from '../../i18n'
defineProps<{ records: ApprovalRecord[]; disabled: boolean; history: boolean }>()
const emit = defineEmits<{ decide: [kind: string, record: ApprovalRecord] }>()
const { t } = useI18n()
</script>
<template>
  <div class="panel">
    <h3>{{ t(history ? 'permission.history' : 'permission.pending') }}</h3>
    <p>{{ t(history ? 'permission.history_bound' : 'permission.pending_truth') }}</p>
    <p v-if="!records.length">{{ t('permission.empty') }}</p>
    <article v-for="record in records" :key="record.approval_id" class="panel">
      <h4>{{ record.tool }} <span v-if="record.action">/ {{ record.action }}</span></h4>
      <p>{{ record.status }} · {{ record.approval_id }}</p>
      <p>{{ record.summary }} · {{ record.scope }}</p>
      <p>{{ t('permission.scope') }}: {{ record.binding.workspace_hash || t('common.unavailable') }} · {{ record.binding.session_hash || t('common.unavailable') }}</p>
      <p>{{ record.reason }}</p>
      <p>{{ t('permission.created') }}: {{ record.created_at }} · {{ t('permission.expires') }}: {{ record.expires_at }}</p>
      <p>{{ t('permission.execution') }}: {{ record.dispatch_outcome }}</p>
      <details>
        <summary>{{ t('permission.trace') }}</summary>
        <p v-if="!record.decision">{{ t('common.unavailable') }}</p>
        <template v-else>
          <p>{{ record.decision.effect }} · {{ record.decision.reason }} · {{ record.decision.effective.mode }}</p>
          <p>{{ t('permission.filesystem') }}: {{ record.decision.effective.settings.permission_profile.filesystem }} · {{ t('permission.network') }}: {{ record.decision.effective.settings.permission_profile.network }} · {{ t('permission.workspace_boundary') }}: {{ record.decision.effective.settings.permission_profile.sandbox_boundary }}</p>
          <ul><li v-for="(source, index) in record.decision.sources" :key="index">{{ source.kind }} · {{ source.effect }} · {{ source.reason }}</li></ul>
        </template>
        <p>{{ t('permission.constraints') }}</p>
      </details>
      <template v-if="!history">
        <div class="actions">
          <button type="button" :disabled="disabled || !record.can_approve_once" @click="emit('decide', 'approve_once', record)">{{ t('permission.once') }}</button>
          <button type="button" :disabled="disabled || !record.can_approve_workspace" @click="emit('decide', 'approve_workspace', record)">{{ t('permission.workspace') }}</button>
          <button type="button" :disabled="disabled" @click="emit('decide', 'reject', record)">{{ t('permission.reject') }}</button>
        </div>
        <p v-if="!record.can_approve_once">{{ t('permission.once_unavailable') }}</p>
        <p v-if="!record.can_approve_workspace">{{ t('common.unavailable') }}: {{ record.workspace_unavailable_reason || t('permission.workspace_unavailable') }}</p>
      </template>
    </article>
  </div>
</template>
