<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api, hasConnected, subscribe, type Device, type Entity, type ServerStatus, type WidgetKind } from './api'
import AddDisplay from './components/AddDisplay.vue'
import DeviceView from './components/DeviceView.vue'
import Editor from './components/Editor.vue'
import HaSettings from './components/HaSettings.vue'

type View = 'layout' | 'display'

const status = ref<ServerStatus>()
const devices = ref<Device[]>([])
const kinds = ref<WidgetKind[]>([])
const entities = ref<Entity[]>([])
const selectedId = ref<string>()
const view = ref<View>('layout')
const previewVersion = ref(0)
const loaded = ref(false)
const serverLost = ref(false)
const loadError = ref('')
const adding = ref(false)
const haOpen = ref(false)

const selected = computed(() => devices.value.find((d) => d.id === selectedId.value))

// ---- Address bar ---------------------------------------------------------
// #/<device>/<view> keeps the place across reloads and makes it linkable.
function readHash() {
  const [, id, v] = location.hash.split('/')
  if (id) selectedId.value = decodeURIComponent(id)
  view.value = v === 'display' ? 'display' : 'layout'
}
watch([selectedId, view], ([id, v]) => {
  const hash = id ? `#/${encodeURIComponent(id)}${v === 'display' ? '/display' : ''}` : ''
  if (hash !== location.hash) history.replaceState(null, '', hash || location.pathname)
})

// ---- Data ----------------------------------------------------------------
async function loadDevices() {
  devices.value = await api.devices()
  if (!selected.value) selectedId.value = devices.value[0]?.id
}

async function refreshDevice(id: string) {
  const d = await api.device(id)
  const i = devices.value.findIndex((x) => x.id === id)
  if (i >= 0) devices.value[i] = { ...d, layout: devices.value[i].layout }
}

async function loadSource() {
  ;[status.value, entities.value] = await Promise.all([api.status(), api.entities()])
}

async function loadAll() {
  try {
    ;[kinds.value] = await Promise.all([api.widgets(), loadSource(), loadDevices()])
    loadError.value = ''
    loaded.value = true
    previewVersion.value++
  } catch (e) {
    loadError.value = (e as Error).message
  }
}

function quietly(p: Promise<unknown>) {
  p.catch(() => {
    /* the reconnect banner covers a lost server */
  })
}

let stop: (() => void) | undefined
let entityTimer: number | undefined
onMounted(() => {
  readHash()
  window.addEventListener('hashchange', readHash)
  stop = subscribe(
    (e) => {
      if (e.type === 'devices') quietly(loadDevices())
      else if (e.type === 'device') quietly(refreshDevice(e.device))
      else if (e.type === 'render' && e.device === selectedId.value) previewVersion.value++
      else if (e.type === 'settings') onSourceChanged()
    },
    () => {
      serverLost.value = false
      loadAll()
    },
    () => (serverLost.value = true),
  )
  // Entity states feed the pickers; refresh them now and then.
  entityTimer = window.setInterval(() => quietly(loadSource()), 20000)
})
onUnmounted(() => {
  stop?.()
  clearInterval(entityTimer)
  window.removeEventListener('hashchange', readHash)
})

// After the Home Assistant connection changed, entities arrive a moment later.
function onSourceChanged() {
  for (const ms of [0, 1500, 4000])
    setTimeout(() => quietly(loadSource().then(() => previewVersion.value++)), ms)
}

function onCreated(d: Device) {
  if (!devices.value.some((x) => x.id === d.id)) devices.value.push(d)
  selectedId.value = d.id
  view.value = 'layout'
}

function onDeleted(id: string) {
  devices.value = devices.value.filter((d) => d.id !== id)
  selectedId.value = devices.value[0]?.id
  view.value = 'layout'
}

function onUpdated(d: Device) {
  const i = devices.value.findIndex((x) => x.id === d.id)
  if (i >= 0) devices.value[i] = d
}

function dotClass(d: Device) {
  if (!d.status.connected) return ''
  return d.status.in_sync || !d.status.last_ack_seq ? 'ok' : 'warn'
}

function stateWord(d: Device) {
  if (d.status.connected) return 'online'
  return hasConnected(d) ? 'offline' : 'not connected yet'
}

const ha = computed(() => {
  const s = status.value
  if (!s) return undefined
  if (s.source === 'demo') return { cls: '', text: 'Demo data', action: 'Connect Home Assistant' }
  if (s.connected) return { cls: 'ok', text: 'Home Assistant', action: '' }
  return { cls: 'bad', text: 'Home Assistant unreachable', action: '' }
})
</script>

<template>
  <div class="app">
    <header>
      <h1>homepaper</h1>

      <nav class="displays" aria-label="Displays">
        <button
          v-for="d in devices"
          :key="d.id"
          class="tab"
          :class="{ active: d.id === selectedId }"
          :aria-current="d.id === selectedId ? 'page' : undefined"
          :title="`${d.name}: ${stateWord(d)}`"
          @click="selectedId = d.id"
        >
          <span class="dot" :class="dotClass(d)"></span>
          <span class="tab-name">{{ d.name }}</span>
        </button>
        <button v-if="loaded" class="quiet add" @click="adding = true">+ Add display</button>
      </nav>

      <button v-if="ha" class="quiet ha" title="Home Assistant connection" @click="haOpen = true">
        <span class="dot" :class="ha.cls"></span>
        <span>{{ ha.text }}</span>
        <span v-if="ha.action" class="ha-action">{{ ha.action }}</span>
      </button>
    </header>

    <p v-if="serverLost && loaded" class="banner" role="status">
      Lost the connection to the homepaper server. Trying again…
    </p>
    <p v-else-if="loadError" class="banner" role="alert">Couldn't load from the server: {{ loadError }}</p>

    <main v-if="selected">
      <div class="title">
        <div>
          <h2>{{ selected.name }}</h2>
          <button class="quiet state" title="Show connection details" @click="view = 'display'">
            <span class="dot" :class="dotClass(selected)"></span>
            {{ stateWord(selected) }}
            <span v-if="!hasConnected(selected) && view === 'layout'" class="connect">Connect the display</span>
          </button>
        </div>
        <div class="views" role="tablist">
          <button role="tab" :aria-selected="view === 'layout'" :class="{ on: view === 'layout' }" @click="view = 'layout'">
            Layout
          </button>
          <button role="tab" :aria-selected="view === 'display'" :class="{ on: view === 'display' }" @click="view = 'display'">
            Display
          </button>
        </div>
      </div>

      <Editor
        v-if="view === 'layout'"
        :key="selected.id"
        :device="selected"
        :kinds="kinds"
        :entities="entities"
        :preview-version="previewVersion"
        @saved="onUpdated"
      />
      <DeviceView v-else :key="'d-' + selected.id" :device="selected" @updated="onUpdated" @deleted="onDeleted" />
    </main>

    <main v-else-if="loaded" class="first">
      <h2>Lay out your first display</h2>
      <p>
        Add a display to start arranging widgets. You can design the layout right away and connect the e-ink panel
        whenever it's ready.
      </p>
      <button class="primary" @click="adding = true">Add a display</button>
    </main>

    <AddDisplay v-if="adding" :existing="devices.map((d) => d.id)" @close="adding = false" @created="onCreated" />
    <HaSettings v-if="haOpen" @close="haOpen = false" @changed="onSourceChanged" />
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
}

header {
  display: flex;
  align-items: stretch;
  gap: 24px;
  min-height: 52px;
  padding: 0 24px;
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}

h1 {
  align-self: center;
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.displays {
  flex: 1;
  display: flex;
  align-items: stretch;
  gap: 2px;
  min-width: 0;
  overflow-x: auto;
}

.tab {
  flex: none;
  max-width: 220px;
  min-height: 0;
  padding: 0 12px;
  border: none;
  border-radius: 0;
  border-bottom: 3px solid transparent;
  border-top: 3px solid transparent;
  background: none;
  color: var(--muted);
}

.tab.active {
  border-bottom-color: var(--ink);
  color: var(--ink);
}

.tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.add,
.ha {
  flex: none;
  align-self: center;
}

.add {
  color: var(--blue);
}

.ha-action {
  color: var(--blue);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.banner {
  padding: 8px 24px;
  background: var(--warn-wash);
  border-bottom: 1px solid var(--rule);
  font-size: var(--text-s);
}

main {
  max-width: 1240px;
  margin: 0 auto;
  padding: 24px;
}

.title {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 14px;
}

.title > div:first-child {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 12px;
}

.state {
  min-height: 0;
  padding: 2px 6px;
  font-size: var(--text-s);
  font-weight: 400;
  color: var(--muted);
}

/* A two-way switch: the pressed side is the sheet, the other the desk. */
.views {
  display: flex;
  padding: 2px;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: var(--rule-soft);
}

.views button {
  min-height: 28px;
  padding: 2px 16px;
  border: none;
  border-radius: 3px;
  background: none;
  color: var(--muted);
}

.views button.on {
  background: var(--sheet);
  color: var(--ink);
}

.connect {
  color: var(--blue);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.first {
  max-width: 520px;
  margin: 12vh auto 0;
}

.first p {
  margin: 10px 0 20px;
  color: var(--muted);
}

@media (max-width: 700px) {
  header {
    flex-wrap: wrap;
    gap: 0 16px;
    padding: 0 16px;
  }

  h1 {
    min-height: 46px;
    display: flex;
    align-items: center;
  }

  .ha {
    margin-left: auto;
  }

  .displays {
    order: 3;
    flex-basis: 100%;
    min-height: 42px;
    border-top: 1px solid var(--rule-soft);
  }

  main {
    padding: 16px;
  }
}
</style>
