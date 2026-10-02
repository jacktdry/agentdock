<script setup lang="ts">
import EventStreamPanel from './components/events/EventStreamPanel.vue'
import RuntimeStatusPanel from './components/runtime/RuntimeStatusPanel.vue'
import PocSettingsPanel from './components/settings/PocSettingsPanel.vue'
</script>

<template>
  <main class="app-shell">
    <header class="hero">
      <div>
        <p class="eyebrow">M1 architecture spike · Wails v3 beta.27</p>
        <h1>AgentDock shared desktop POC</h1>
        <p class="lede">
          One Vue UI, direct Go services, bounded event delivery, native menu and tray.
        </p>
      </div>
      <div class="poc-badge" aria-label="Proof of concept only">POC</div>
    </header>

    <div class="layout-grid">
      <RuntimeStatusPanel />
      <PocSettingsPanel />
      <EventStreamPanel />
    </div>
  </main>
</template>
