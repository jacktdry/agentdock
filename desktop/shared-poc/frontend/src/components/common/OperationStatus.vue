<script setup lang="ts">
import type { APIError } from '../../api/desktopApi'
import { errorMessage } from '../../api/errorMessage'
import { useI18n } from '../../i18n'
defineProps<{ busy: boolean; error: APIError | null; completed?: boolean }>()
const { t } = useI18n()
</script>
<template>
  <p aria-live="polite" :class="{ error: !!error }">
    {{ busy ? t('common.loading') : error ? errorMessage(error) : completed ? t('common.completed') : '' }}
  </p>
</template>
