<script setup lang="ts">
import { onBeforeUnmount, onMounted, useTemplateRef } from 'vue'
import { useI18n } from '../../i18n'
defineProps<{ prompt: string }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const { t } = useI18n()
const returnFocus = typeof document === 'undefined' ? null : document.activeElement as HTMLElement | null
onMounted(() => dialog.value?.showModal())
onBeforeUnmount(() => {
  dialog.value?.close()
  if (returnFocus?.isConnected) returnFocus.focus()
})
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
<style scoped>
dialog { overflow-wrap: anywhere; box-sizing: border-box; }
</style>
