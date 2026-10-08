<script setup lang="ts">
import PermissionPanel from './components/permission/PermissionPanel.vue'
import ACPPanel from './components/acp/ACPPanel.vue'
import MCPPanel from './components/mcp/MCPPanel.vue'
import PluginPanel from './components/plugin/PluginPanel.vue'
import ActivityStreamPanel from './components/activity/ActivityStreamPanel.vue'
import ExecutionCenterPanel from './components/execution/ExecutionCenterPanel.vue'
import ContractStatusPanel from './components/contract/ContractStatusPanel.vue'
import RuntimeStatusPanel from './components/runtime/RuntimeStatusPanel.vue'
import { shallowRef } from 'vue'
import BasicSettingsPanel from './components/settings/BasicSettingsPanel.vue'
import ConnectionWorkspace from './components/connection/ConnectionWorkspace.vue'
import OverviewPanel from './components/overview/OverviewPanel.vue'
import UpdateCard from './components/system/UpdateCard.vue'
import DiagnosticsCard from './components/system/DiagnosticsCard.vue'
import { primarySections, sectionKeys, type Section } from './navigation'
const section = shallowRef<Section>('overview')
const pluginTarget = shallowRef('')
import { type LocalePreference, useI18n } from './i18n'

const { currentPreference, localeOptions, setLocale, t } = useI18n()
const productName = import.meta.env.VITE_AGENTDOCK_PRODUCT_NAME
if (productName) document.title = productName

function changeLocale(event: Event) {
  setLocale((event.target as HTMLSelectElement).value as LocalePreference)
}

function navigate(next: Section) {
  if (next !== 'plugin') pluginTarget.value = ''
  section.value = next
}

function openPlugin(name: string) {
  pluginTarget.value = name
  section.value = 'plugin'
}
</script>

<template>
  <div class="app-shell">
    <header class="hero">
      <div>
        <p class="eyebrow">{{ t('app.eyebrow') }}</p>
        <h1>{{ productName || t('app.title') }}</h1>
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
      </div>
    </header>

    <div class="desktop-layout">
      <nav class="primary-nav" :aria-label="t('nav.primary')">
        <button v-for="item in primarySections" :key="item" type="button"
          :aria-current="section === item ? 'page' : undefined" @click="navigate(item)">{{ t(sectionKeys[item]) }}</button>
        <button class="developer-entry" type="button" :aria-current="section === 'developer' ? 'page' : undefined"
          @click="section = 'developer'">{{ t('nav.developer') }}</button>
      </nav>
      <main class="feature-content">
        <OverviewPanel v-if="section === 'overview'" @navigate="navigate" />
        <RuntimeStatusPanel v-else-if="section === 'runtime'" />
        <ExecutionCenterPanel v-else-if="section === 'execution'" />
        <ConnectionWorkspace v-else-if="section === 'connection'" />
        <ACPPanel v-else-if="section === 'acp'" />
        <MCPPanel v-else-if="section === 'mcp'" @manage-plugin="openPlugin" />
        <PluginPanel v-else-if="section === 'plugin'" :focus-name="pluginTarget" @open-mcp="navigate('mcp')" />
        <PermissionPanel v-else-if="section === 'permission'" />
        <BasicSettingsPanel v-else-if="section === 'settings'" />
        <div v-else-if="section === 'system'" class="feature-stack">
          <UpdateCard />
          <DiagnosticsCard />
        </div>
        <section v-else class="feature-stack" aria-labelledby="developer-title">
          <h2 id="developer-title">{{ t('nav.developer') }}</h2>
          <p class="hint">{{ t('developer.scope') }}</p>
          <ContractStatusPanel />
          <p class="hint">{{ t('developer.activity_experimental') }}</p>
          <ActivityStreamPanel />
        </section>
      </main>
    </div>
  </div>
</template>
