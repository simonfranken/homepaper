<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { api, subscribe, type Device, type Entity, type ServerStatus, type WidgetKind } from './api'
import DeviceList from './components/DeviceList.vue'
import Editor from './components/Editor.vue'
import DevicePanel from './components/DevicePanel.vue'

const status = ref<ServerStatus>()
const devices = ref<Device[]>([])
const kinds = ref<WidgetKind[]>([])
const entities = ref<Entity[]>([])
const selectedId = ref<string>()
const previewVersion = ref(0)

const selected = computed(() => devices.value.find((d) => d.id === selectedId.value))

async function loadDevices() {
  devices.value = await api.devices()
  if (!selected.value) selectedId.value = devices.value[0]?.id
}

async function refreshDevice(id: string) {
  const d = await api.device(id)
  const i = devices.value.findIndex((x) => x.id === id)
  if (i >= 0) devices.value[i] = { ...d, layout: devices.value[i].layout }
}

async function loadAll() {
  ;[status.value, kinds.value, entities.value] = await Promise.all([api.status(), api.widgets(), api.entities()])
  await loadDevices()
  previewVersion.value++
}

let stop: (() => void) | undefined
let entityTimer: number | undefined
onMounted(() => {
  stop = subscribe(
    (e) => {
      if (e.type === 'devices') loadDevices()
      else if (e.type === 'device') refreshDevice(e.device)
      else if (e.type === 'render' && e.device === selectedId.value) previewVersion.value++
    },
    () => loadAll(),
  )
  // Entity list and HA status change rarely; poll them gently.
  entityTimer = window.setInterval(async () => {
    ;[status.value, entities.value] = await Promise.all([api.status(), api.entities()])
  }, 30000)
})
onUnmounted(() => {
  stop?.()
  clearInterval(entityTimer)
})

function onCreated(d: Device) {
  devices.value.push(d)
  selectedId.value = d.id
}

function onDeleted(id: string) {
  devices.value = devices.value.filter((d) => d.id !== id)
  selectedId.value = devices.value[0]?.id
}

function onUpdated(d: Device) {
  const i = devices.value.findIndex((x) => x.id === d.id)
  if (i >= 0) devices.value[i] = d
}
</script>

<template>
  <div class="app">
    <header>
      <div class="brand">
        <h1>homepaper</h1>
        <span class="muted tagline">the newspaper of your smart home</span>
      </div>
      <div v-if="status" class="ha">
        <span class="dot" :class="status.connected ? 'ok' : 'bad'"></span>
        <span v-if="status.source === 'demo'">Demo data ({{ status.entities }} entities)</span>
        <span v-else>Home Assistant {{ status.connected ? 'connected' : 'disconnected' }} · {{ status.entities }} entities</span>
      </div>
    </header>

    <div class="body">
      <aside>
        <DeviceList :devices="devices" :selected="selectedId" @select="selectedId = $event" @created="onCreated" />
      </aside>

      <main v-if="selected">
        <Editor
          :key="selected.id"
          :device="selected"
          :kinds="kinds"
          :entities="entities"
          :preview-version="previewVersion"
          @saved="onUpdated"
        />
        <DevicePanel :device="selected" @updated="onUpdated" @deleted="onDeleted" />
      </main>
      <main v-else class="empty card">
        <h2>No display yet</h2>
        <p class="muted">Add a device on the left. You will get a device ID and token to enter in the display's setup page.</p>
      </main>
    </div>
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
}

header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 14px 20px;
  border-bottom: 3px double var(--ink);
  background: var(--paper);
}

.brand {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
}

.brand h1 {
  font-size: 28px;
  letter-spacing: -0.5px;
}

.tagline {
  font-style: italic;
  font-family: Georgia, serif;
}

.ha {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.body {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 20px;
  padding: 20px;
  max-width: 1400px;
  margin: 0 auto;
}

main {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}

.empty {
  justify-content: center;
}

@media (max-width: 800px) {
  .body {
    grid-template-columns: 1fr;
    padding: 16px;
  }
}
</style>
