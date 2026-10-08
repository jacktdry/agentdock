<script setup lang="ts">
import { shallowRef } from 'vue'
import { useI18n } from '../../i18n'
import ConnectionPanel from './ConnectionPanel.vue'
import NexusDockPanel from './NexusDockPanel.vue'
const view = shallowRef<'chatgpt' | 'nexus'>('chatgpt')
const { t } = useI18n()
</script>
<template>
  <div>
    <nav class="actions connection-subnav" :aria-label="t('nexus.navigation')">
      <button type="button" :aria-current="view === 'chatgpt' ? 'page' : undefined" @click="view = 'chatgpt'">{{ t('nexus.chatgpt_access') }}</button>
      <button type="button" :aria-current="view === 'nexus' ? 'page' : undefined" @click="view = 'nexus'">{{ t('nexus.title') }}</button>
    </nav>
    <ConnectionPanel v-if="view === 'chatgpt'" />
    <NexusDockPanel v-else />
  </div>
</template>
<style scoped>
.connection-subnav { margin-bottom: 1rem; }
.connection-subnav [aria-current="page"] { background: #293c5b; border-color: #8fb3ff; }
</style>
