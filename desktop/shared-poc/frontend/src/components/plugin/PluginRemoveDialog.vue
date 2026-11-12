<script setup lang="ts">
import { onMounted, useTemplateRef } from 'vue'
import { useI18n } from '../../i18n'

defineProps<{ pluginName: string; busy: boolean }>()
const emit = defineEmits<{ keep: []; purge: []; cancel: [] }>()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const { t } = useI18n()
onMounted(() => dialog.value?.showModal())
</script>

<template>
  <dialog ref="dialog" class="plugin-remove-dialog" aria-labelledby="plugin-remove-title" @cancel.prevent="emit('cancel')">
    <h2 id="plugin-remove-title">{{ t('plugin.remove_title') }}</h2>
    <p class="execution-meta">{{ pluginName }}</p>

    <div class="plugin-remove-options">
      <section>
        <h3>{{ t('plugin.remove_keep') }}</h3>
        <p>{{ t('plugin.remove_keep_description') }}</p>
        <button type="button" :disabled="busy" @click="emit('keep')">{{ t('plugin.remove_keep') }}</button>
      </section>
      <section class="danger-zone">
        <h3>{{ t('plugin.remove_purge') }}</h3>
        <p>{{ t('plugin.remove_purge_description') }}</p>
        <button type="button" class="danger-button" :disabled="busy" @click="emit('purge')">{{ t('plugin.remove_purge') }}</button>
      </section>
    </div>

    <div class="actions">
      <button type="button" autofocus :disabled="busy" @click="emit('cancel')">{{ t('common.cancel') }}</button>
    </div>
  </dialog>
</template>
