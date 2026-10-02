<script setup lang="ts">
import ActivityStreamPanel from './components/activity/ActivityStreamPanel.vue'
import ContractStatusPanel from './components/contract/ContractStatusPanel.vue'
import RuntimeStatusPanel from './components/runtime/RuntimeStatusPanel.vue'
import PocSettingsPanel from './components/settings/PocSettingsPanel.vue'
</script>

<template>
  <main class="app-shell">
    <header class="hero">
      <div>
        <p class="eyebrow">M2 shared desktop API · Wails v3 beta.27</p>
        <h1>AgentDock shared desktop API</h1>
        <p class="lede">
          Versioned control contracts, explicit capabilities and bounded Activity delivery over one Vue UI.
        </p>
      </div>
      <div class="poc-badge" aria-label="Architecture proof of concept">M2</div>
    </header>

    <div class="layout-grid">
      <RuntimeStatusPanel />
      <ContractStatusPanel />
      <PocSettingsPanel />
      <ActivityStreamPanel />
    </div>
  </main>
</template>
