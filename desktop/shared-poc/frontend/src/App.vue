<script setup lang="ts">
import ActivityStreamPanel from './components/activity/ActivityStreamPanel.vue'
import ContractStatusPanel from './components/contract/ContractStatusPanel.vue'
import RuntimeStatusPanel from './components/runtime/RuntimeStatusPanel.vue'
import PocSettingsPanel from './components/settings/PocSettingsPanel.vue'
import { type LocalePreference, useI18n } from './i18n'

const { currentPreference, localeOptions, setLocale, t } = useI18n()

function changeLocale(event: Event) {
  setLocale((event.target as HTMLSelectElement).value as LocalePreference)
}
</script>

<template>
  <main class="app-shell">
    <header class="hero">
      <div>
        <p class="eyebrow">{{ t('app.eyebrow') }}</p>
        <h1>{{ t('app.title') }}</h1>
        <p class="lede">{{ t('app.lede') }}</p>
      </div>
      <div class="hero-actions">
        <label class="language-picker">
          <span class="sr-only">{{ t('settings.language') }}</span>
          <select :value="currentPreference" @change="changeLocale">
            <option value="system">{{ t('common.follow_system') }}</option>
            <option v-for="locale in localeOptions" :key="locale.code" :value="locale.code">
              {{ locale.nativeName }}
            </option>
          </select>
        </label>
        <div class="poc-badge" :aria-label="t('app.poc_label')">M3</div>
      </div>
    </header>

    <div class="layout-grid">
      <RuntimeStatusPanel />
      <ContractStatusPanel />
      <PocSettingsPanel />
      <ActivityStreamPanel />
    </div>
  </main>
</template>
