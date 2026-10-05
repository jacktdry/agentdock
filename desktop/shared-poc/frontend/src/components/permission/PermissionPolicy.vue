<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ApprovalCategories, Policy } from '../../../bindings/github.com/uvwt/agentdock/internal/permission/models'
import { useI18n } from '../../i18n'
const props = defineProps<{ policy: Policy; disabled: boolean }>()
const emit = defineEmits<{ save: [policy: Policy] }>()
const { t } = useI18n()
const mode = ref('on-request')
const categories = ref<ApprovalCategories>({ file_writes: false, commands: false, network: false, mcp: false, management: false, other: false })
const keys: (keyof ApprovalCategories)[] = ['file_writes', 'commands', 'network', 'mcp', 'management', 'other']
watch(() => props.policy.revision, () => {
  const policy = props.policy
  mode.value = policy.settings.approval_policy.mode
  for (const key of keys) categories.value[key] = policy.settings.approval_policy.granular?.[key] ?? false
}, { immediate: true })
function save() {
  const policy: Policy = JSON.parse(JSON.stringify(props.policy))
  policy.settings.approval_policy = { mode: mode.value, ...(mode.value === 'granular' ? { granular: { ...categories.value } } : {}) }
  emit('save', policy)
}
</script>
<template>
  <div class="panel">
    <h3>{{ t('permission.profile') }}</h3>
    <dl>
      <dt>{{ t('permission.filesystem') }}</dt><dd>{{ policy.settings.permission_profile.filesystem }}</dd>
      <dt>{{ t('permission.network') }}</dt><dd>{{ policy.settings.permission_profile.network }}</dd>
      <dt>{{ t('permission.workspace_boundary') }}</dt><dd>{{ policy.settings.permission_profile.sandbox_boundary }}</dd>
      <dt>{{ t('permission.mode') }}</dt><dd>{{ policy.global_mode }}</dd>
      <dt>{{ t('permission.reviewer') }}</dt><dd>{{ policy.settings.approval_reviewer }}</dd>
    </dl>
    <form @submit.prevent="save">
      <fieldset :disabled="disabled">
        <legend>{{ t('permission.policy') }}</legend>
        <label>{{ t('permission.mode') }} <select v-model="mode"><option>on-request</option><option>never</option><option>granular</option></select></label>
        <p>{{ t('permission.never') }}</p>
        <div v-if="mode === 'granular'" class="actions">
          <label v-for="key in keys" :key="key"><input v-model="categories[key]" type="checkbox">{{ t(`permission.${key}`) }}</label>
        </div>
        <button type="submit">{{ t('permission.review_policy') }}</button>
      </fieldset>
    </form>
  </div>
</template>
