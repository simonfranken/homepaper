<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ago, api, hasConnected, logout, subscribe, type Device, type Entity, type Me, type ServerStatus, type WidgetKind } from './api'
import AddDisplay from './components/AddDisplay.vue'
import DeviceView from './components/DeviceView.vue'
import Editor from './components/Editor.vue'
import Icon from './components/Icon.vue'

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
const me = ref<Me>()

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
    ;[kinds.value, me.value] = await Promise.all([api.widgets(), api.me(), loadSource(), loadDevices()])
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

// Stop the event stream and timers first, so nothing reaches the closed API
// and starts a new login on the way out.
async function signOut() {
  stop?.()
  clearInterval(entityTimer)
  await logout()
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

// Fill carries the state: solid in sync, half catching up, hollow
// offline, dotted not connected yet.
function dotClass(d: Device) {
  if (!d.status.connected) return hasConnected(d) ? '' : 'never'
  return d.status.in_sync || !d.status.last_ack_seq ? 'ok' : 'warn'
}

function stateLine(d: Device) {
  if (!d.status.connected) return hasConnected(d) ? 'Offline' : 'Not connected yet'
  if (!d.status.last_ack_seq) return 'Online · sending the first frame'
  return d.status.in_sync ? 'Online · in sync' : 'Online · catching up'
}

function stateWord(d: Device) {
  if (d.status.connected) return 'online'
  return hasConnected(d) ? 'offline' : 'not connected yet'
}

const ha = computed(() => {
  const s = status.value
  if (!s) return undefined
  if (s.source === 'demo') return { cls: 'demo', text: 'Demo data', count: '', title: 'No HA_URL set: showing demo entities' }
  if (s.connected) return { cls: 'ok', text: 'Home Assistant', count: `${s.entities} entities`, title: 'Connected to Home Assistant' }
  return { cls: 'bad', text: 'Home Assistant unreachable', count: '', title: s.error || 'Not connected' }
})
</script>

<template>
  <div class="app">
    <header>
      <h1>
        <svg width="28" height="20" viewBox="0 0 28 20" aria-hidden="true">
          <rect x="1" y="1" width="26" height="18" rx="4" fill="none" stroke="currentColor" stroke-width="2" />
          <rect x="5" y="5" width="8" height="10" rx="1.5" fill="currentColor" />
          <path d="M16 6h7M16 10h7M16 14h4" stroke="currentColor" stroke-width="2" />
        </svg>
        homepaper
      </h1>
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
        <button v-if="loaded" class="quiet small add" @click="adding = true">
          <Icon name="plus" :size="14" :stroke="3" />
          Add display
        </button>
      </nav>
      <p v-if="ha" class="ha" :class="{ bad: ha.cls === 'bad' }" :title="ha.title" role="status">
        <span class="dot" :class="ha.cls"></span>
        <span>{{ ha.text }}</span>
        <span v-if="ha.count" class="count">{{ ha.count }}</span>
      </p>
      <form v-if="me?.auth" class="who" method="post" action="/auth/logout" @submit.prevent="signOut">
        <span class="user" :title="me.admin ? 'Home Assistant administrator' : 'Home Assistant user'">{{ me.name }}</span>
        <button type="submit" class="quiet small">Log out</button>
      </form>
    </header>
    <p v-if="serverLost && loaded" class="banner lost" role="status">
      <span><span class="spin"></span> Lost the connection to the homepaper server. Trying again…</span>
    </p>
    <p v-else-if="loadError" class="banner failed" role="alert">
      <Icon name="warn" :size="18" />
      Couldn't load from the server: {{ loadError }}
    </p>
    <main v-if="selected">
      <div class="title">
        <div class="name">
          <h2>{{ selected.name }}</h2>
          <button class="quiet state" title="Show connection details" @click="view = 'display'">
            <span class="dot" :class="dotClass(selected)"></span>
            <span>{{ stateLine(selected) }}</span>
            <span v-if="selected.status.connected && ago(selected.status.last_ack_at)" class="muted">
              · last frame {{ ago(selected.status.last_ack_at) }}
            </span>
            <span v-if="!hasConnected(selected) && view === 'layout'" class="connect">Connect the display</span>
          </button>
        </div>
        <div class="views" role="tablist" aria-label="View">
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
        @open-display="view = 'display'"
      />
      <DeviceView
        v-else
        :key="'d-' + selected.id"
        :device="selected"
        @updated="onUpdated"
        @deleted="onDeleted"
        @open-layout="view = 'layout'"
      />
    </main>
    <main v-else-if="loaded" class="first">
      <div class="mini-bezel" aria-hidden="true">
        <div class="mini-panel">Hi.</div>
      </div>
      <div class="first-text">
        <h2>Lay out your first display</h2>
        <p>
          Add a display to start arranging widgets. You can design the layout right away and connect the e-ink panel
          whenever it's ready.
        </p>
        <button class="primary" @click="adding = true">Add a display</button>
      </div>
    </main>
    <AddDisplay v-if="adding" :existing="devices.map((d) => d.id)" @close="adding = false" @created="onCreated" />
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
}

header {
  display: flex;
  flex-wrap: wrap;
  align-items: stretch;
  gap: 8px 24px;
  min-height: 60px;
  padding: 0 24px;
  border-bottom: 2px solid var(--ink);
  background: var(--sheet);
}

h1 {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: var(--t25);
  font-weight: 700;
  letter-spacing: -0.02em;
}

.displays {
  flex: 1 1 320px;
  display: flex;
  align-items: stretch;
  gap: 2px;
  min-width: 0;
  overflow-x: auto;
}

.tab {
  flex: none;
  max-width: 220px;
  min-height: 56px;
  padding: 0 14px;
  border: 0;
  border-radius: 0;
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
  background: none;
  color: var(--muted);
}

.tab:hover:not(:disabled) {
  background: none;
  color: var(--ink);
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
  margin-left: 6px;
}

.ha.bad {
  color: var(--bad);
}

.count {
  font-family: 'Go Mono', ui-monospace, monospace;
  font-weight: 400;
  color: var(--muted);
}

.ha {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: var(--t12);
  font-weight: 500;
}

.who {
  flex: none;
  align-self: center;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding-left: 16px;
  border-left: 2px dotted var(--ink);
}

.user {
  font-weight: 700;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.banner {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 16px 24px 0;
  padding: 10px 14px;
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  font-weight: 500;
}

/* A 1-bit screen behind the words, not a grey bar. */
.banner.lost {
  background-color: var(--sheet);
  background-image: radial-gradient(#111 0.7px, transparent 0.9px);
  background-size: 4px 4px;
}

.banner.lost > span {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--sheet);
}

.banner.failed {
  border-color: var(--bad);
  background: var(--sheet);
  color: var(--bad);
}

main {
  max-width: 1360px;
  margin: 0 auto;
  padding: 28px 24px 48px;
}

.title {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 24px;
}

.name {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.name h2 {
  font-size: var(--t34);
  line-height: 1.1;
  letter-spacing: -0.015em;
}

.state {
  align-self: flex-start;
  min-height: 28px;
  margin-left: -10px;
  padding: 0 10px;
  font-weight: 400;
  flex-wrap: wrap;
  white-space: normal;
}

.connect {
  color: var(--blue);
  font-weight: 700;
  text-decoration: underline;
  text-underline-offset: 3px;
}

/* A two-way switch: the chosen side is solid ink. */
.views {
  display: inline-flex;
  gap: 3px;
  padding: 3px;
  border: 2px solid var(--ink);
  border-radius: 10px;
  background: var(--sheet);
}

.views button {
  min-height: 36px;
  padding: 0 18px;
  border: 0;
  border-radius: 6px;
  background: none;
  color: var(--ink);
}

.views button:hover:not(:disabled) {
  background: var(--paper);
  color: var(--ink);
}

.views button.on,
.views button.on:hover:not(:disabled) {
  background: var(--ink);
  color: var(--on-ink);
}

.first {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 48px;
  max-width: 1000px;
  margin-top: 10vh;
}

.mini-bezel {
  padding: 10px;
  background: var(--bezel);
  border-radius: 10px;
}

.mini-panel {
  display: grid;
  place-items: center;
  width: 250px;
  aspect-ratio: 800 / 480;
  background-color: #fff;
  background-image:
    linear-gradient(90deg, rgb(17 17 17 / 0.12) 1px, transparent 1px),
    linear-gradient(0deg, rgb(17 17 17 / 0.12) 1px, transparent 1px);
  background-size: 12.5% 16.6667%;
  font-size: var(--t34);
  font-weight: 700;
  color: #111;
}

.first-text {
  flex: 1 1 360px;
  max-width: 520px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 12px;
}

.first-text h2 {
  font-size: var(--t34);
  line-height: 1.1;
}

.first-text p {
  font-size: var(--t17);
  color: var(--muted);
}

@media (max-width: 700px) {
  header {
    padding: 0 16px;
  }

  .ha {
    margin-left: auto;
  }

  .displays {
    order: 3;
    flex-basis: 100%;
    border-top: 2px dotted var(--ink);
  }

  .tab {
    min-height: 44px;
  }

  main {
    padding: 20px 16px 40px;
  }

  .banner {
    margin: 12px 16px 0;
  }
}
</style>
