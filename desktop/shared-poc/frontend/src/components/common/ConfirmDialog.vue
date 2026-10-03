<script setup lang="ts">
import { onMounted, useTemplateRef } from 'vue'
import { useI18n } from '../../i18n'
defineProps<{ prompt: string }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const { t } = useI18n()
onMounted(() => dialog.value?.showModal())
</script>
<template>
  <dialog ref="dialog" aria-labelledby="confirmation-title" @cancel.prevent="emit('cancel')">
    <h2 id="confirmation-title">{{ prompt }}</h2>
    <slot />
    <div class="actions">
      <button type="button" autofocus @click="emit('cancel')">{{ t('common.cancel') }}</button>
      <button type="button" @click="emit('confirm')">{{ t('common.confirm') }}</button>
    </div>
  </dialog>
</template>
