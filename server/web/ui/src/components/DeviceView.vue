<script setup lang="ts">
// Everything about one display that isn't its layout: getting it connected,
// how the connection is doing, and renaming or removing it.
import { computed, ref, watch } from 'vue'
import { api, deviceUrl, hasConnected, isLoopback, setupLink, SETUP_PORTAL, SETUP_SSID, type Device } from '../api'

const props = defineProps<{ device: Device }>()
const emit = defineEmits<{ updated: [d: Device]; deleted: [id: string] }>()

const s = computed(() => props.device.status)
const everConnected = computed(() => hasConnected(props.device))

// ---- Connection values ---------------------------------------------------
// The display needs an address it can reach from its Wi-Fi. That is usually
// the one in the browser's address bar, but not when that says "localhost".
const HOST_KEY = 'homepaper.serverHost'
const pageHost = location.host
const host = ref(localStorage.getItem(HOST_KEY) || pageHost)
watch(host, (h) => {
  if (h.trim() && h.trim() !== pageHost) localStorage.setItem(HOST_KEY, h.trim())
  else localStorage.removeItem(HOST_KEY)
})
const hostValue = computed(() => host.value.trim() || pageHost)
const serverUrl = computed(() => deviceUrl(hostValue.value))
const loopback = computed(() => isLoopback(hostValue.value))
const link = computed(() => setupLink(serverUrl.value, props.device))

const showToken = ref(false)
const copied = ref('')
async function copy(label: string, text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = label
  } catch {
    copied.value = ''
    showToken.value = true
    error.value = 'The browser blocked copying. Select the text and copy it by hand.'
  }
  setTimeout(() => (copied.value = ''), 1800)
}

// ---- Status --------------------------------------------------------------
function ago(iso?: string) {
  if (!iso || iso.startsWith('0001')) return ''
  const sec = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
  if (sec < 60) return 'just now'
  if (sec < 3600) return `${Math.round(sec / 60)} min ago`
  if (sec < 86400) return `${Math.round(sec / 3600)} h ago`
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function duration(sec?: number) {
  if (!sec) return '–'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d ? `${d} d ${h} h` : h ? `${h} h ${m} min` : `${m} min`
}

function signal(rssi?: number) {
  if (!rssi) return '–'
  const word = rssi >= -60 ? 'Strong' : rssi >= -72 ? 'Good' : rssi >= -82 ? 'Weak' : 'Very weak'
  return `${word} (${rssi} dBm)`
}

const state = computed(() => {
  if (!s.value.connected) {
    return everConnected.value
      ? {
          cls: 'bad',
          title: 'Offline',
          text: `Last seen ${ago(props.device.last_seen) || 'a while ago'}. The display keeps showing its last frame and reconnects by itself once it has power and Wi-Fi.`,
        }
      : { cls: '', title: 'Not connected yet', text: 'Follow the steps below to connect the display.' }
  }
  if (!s.value.last_ack_seq)
    return { cls: 'warn', title: 'Connected', text: 'Sending the first frame. A full refresh takes a few seconds.' }
  if (s.value.in_sync)
    return { cls: 'ok', title: 'Online', text: `Showing the current layout. Last update ${ago(s.value.last_ack_at)}.` }
  return {
    cls: 'warn',
    title: 'Online, but the picture may be wrong',
    text: 'The display reported a different checksum than the frame the server sent. A restart of the display usually fixes it.',
  }
})

// ---- Manage --------------------------------------------------------------
const name = ref(props.device.name)
watch(
  () => props.device.name,
  (n) => (name.value = n),
)
const error = ref('')
const confirming = ref<'token' | 'delete'>()

async function run(fn: () => Promise<void>) {
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = (e as Error).message
  }
}

const rename = () =>
  run(async () => {
    const n = name.value.trim()
    if (!n || n === props.device.name) return
    emit('updated', { ...(await api.renameDevice(props.device.id, n)), layout: props.device.layout })
  })

const resetToken = () =>
  run(async () => {
    emit('updated', { ...(await api.resetToken(props.device.id)), layout: props.device.layout })
    confirming.value = undefined
  })

const remove = () =>
  run(async () => {
    await api.deleteDevice(props.device.id)
    emit('deleted', props.device.id)
  })
</script>

<template>
  <div class="view">
    <section class="status">
      <p class="state"><span class="dot" :class="state.cls"></span> {{ state.title }}</p>
      <p class="muted">{{ state.text }}</p>
      <p v-if="s.last_error && !s.in_sync" class="error">{{ s.last_error }}</p>

      <details v-if="everConnected">
        <summary>Details</summary>
        <dl>
          <dt>Firmware</dt>
          <dd>{{ s.firmware || '–' }}</dd>
          <dt>Panel</dt>
          <dd>{{ s.panel || `${device.panel.model} ${device.panel.width}×${device.panel.height}` }}</dd>
          <dt>Address</dt>
          <dd>{{ s.remote || '–' }}</dd>
          <dt>Wi-Fi signal</dt>
          <dd>{{ signal(s.rssi) }}</dd>
          <dt>Running for</dt>
          <dd>{{ duration(s.uptime_s) }}</dd>
          <dt>Free memory</dt>
          <dd>{{ s.free_heap ? Math.round(s.free_heap / 1024) + ' KB' : '–' }}</dd>
          <dt>Frames sent</dt>
          <dd>
            {{ s.frames_sent }}<template v-if="s.frames_sent">
              ({{ (s.bytes_sent / 1024).toFixed(1) }} KB), last one {{ s.last_kind }}</template
            >
          </dd>
          <dt>Confirmed</dt>
          <dd>{{ s.last_ack_seq ? `frame ${s.last_ack_seq} of ${s.last_seq}, ${ago(s.last_ack_at)}` : '–' }}</dd>
        </dl>
      </details>
    </section>

    <section>
      <details class="setup" :open="!everConnected">
      <summary><h3>{{ everConnected ? 'Set it up again' : 'Connect the display' }}</h3></summary>
      <ol class="steps">
        <li>
          <p>
            Power the display. A new display opens the Wi-Fi network <b>{{ SETUP_SSID }}</b
            >.
          </p>
          <p class="muted small">
            One that was set up before does so after you hold its BOOT button for 3 seconds while it runs.
          </p>
        </li>
        <li>
          <p>Check the address the display should reach this server at.</p>
          <div class="host">
            <input v-model="host" aria-label="Server address" spellcheck="false" autocomplete="off" :placeholder="pageHost" />
          </div>
          <p v-if="loopback" class="notice">
            <b>{{ hostValue }}</b> only works on this computer. Enter the server's address on your network instead, for
            example 192.168.1.10:8080.
          </p>
        </li>
        <li>
          <p>
            Join <b>{{ SETUP_SSID }}</b> with this computer or phone, then open the setup page. It arrives filled in; you
            only choose your Wi-Fi.
          </p>
          <p class="muted small">This page stops loading while you're on that network. That's expected.</p>
          <div v-if="!loopback" class="row">
            <a class="button primary" :href="link" target="_blank" rel="noopener">Open the setup page</a>
            <button @click="copy('link', link)">{{ copied === 'link' ? 'Copied' : 'Copy its link' }}</button>
          </div>
          <div v-else class="row">
            <button class="primary" disabled>Open the setup page</button>
            <span class="muted small">Enter a network address in step 2 first.</span>
          </div>
        </li>
        <li>
          <p>Save there. The display restarts, joins your Wi-Fi and shows up here as online.</p>
        </li>
      </ol>

      <details>
        <summary>Enter the values by hand</summary>
        <p class="muted small">Open <span class="mono">{{ SETUP_PORTAL }}</span> on the setup network and type these in.</p>
        <dl class="values">
          <dt>Server URL</dt>
          <dd>
            <span class="mono">{{ serverUrl }}</span>
            <button class="small" @click="copy('url', serverUrl)">{{ copied === 'url' ? 'Copied' : 'Copy' }}</button>
          </dd>
          <dt>Device ID</dt>
          <dd>
            <span class="mono">{{ device.id }}</span>
            <button class="small" @click="copy('id', device.id)">{{ copied === 'id' ? 'Copied' : 'Copy' }}</button>
          </dd>
          <dt>Token</dt>
          <dd>
            <span class="mono">{{ showToken ? device.token : '•'.repeat(16) }}</span>
            <button class="small" @click="showToken = !showToken">{{ showToken ? 'Hide' : 'Show' }}</button>
            <button class="small" @click="copy('token', device.token)">{{ copied === 'token' ? 'Copied' : 'Copy' }}</button>
          </dd>
        </dl>
      </details>
      </details>
    </section>

    <section>
      <h3>Manage</h3>
      <form class="rename" @submit.prevent="rename">
        <label for="dev-name">Name</label>
        <div class="row">
          <input id="dev-name" v-model="name" maxlength="60" autocomplete="off" />
          <button type="submit" :disabled="!name.trim() || name.trim() === device.name">Rename</button>
        </div>
      </form>

      <div class="danger-zone">
        <div v-if="confirming === 'token'" class="confirm">
          <p>The display disconnects until you run its setup again with the new token.</p>
          <div class="row">
            <button class="danger" @click="resetToken">Replace token</button>
            <button @click="confirming = undefined">Cancel</button>
          </div>
        </div>
        <div v-else-if="confirming === 'delete'" class="confirm">
          <p>This removes “{{ device.name }}” and its layout for good.</p>
          <div class="row">
            <button class="danger" @click="remove">Delete display</button>
            <button @click="confirming = undefined">Cancel</button>
          </div>
        </div>
        <div v-else class="row">
          <button title="For when the token may have leaked" @click="confirming = 'token'">Replace token…</button>
          <button class="danger" @click="confirming = 'delete'">Delete display…</button>
        </div>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </section>
  </div>
</template>

<style scoped>
.view {
  max-width: 640px;
  border: 1px solid var(--rule);
  border-radius: 6px;
  background: var(--sheet);
}

section {
  padding: 20px 24px;
}

section + section {
  border-top: 1px solid var(--rule-soft);
}

h3 {
  margin-bottom: 12px;
}

.state {
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: var(--text-l);
  font-weight: 700;
  margin-bottom: 4px;
}

details {
  margin-top: 14px;
}

summary {
  width: fit-content;
  color: var(--blue);
  font-size: var(--text-s);
  font-weight: 500;
  cursor: pointer;
}

details > :not(summary):first-of-type {
  margin-top: 10px;
}

dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 6px 16px;
  margin: 10px 0 0;
  font-size: var(--text-s);
}

dt {
  color: var(--muted);
}

dd {
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.values {
  align-items: center;
}

.setup {
  margin: 0;
}

.setup > summary {
  width: auto;
  list-style: none;
  color: inherit;
  font-size: inherit;
}

.setup > summary::-webkit-details-marker {
  display: none;
}

.setup > summary h3 {
  display: inline;
}

.setup:not([open]) > summary h3 {
  color: var(--blue);
}

.setup:not([open]) > summary h3::after {
  content: ' ▸';
}

.setup[open] > summary {
  margin-bottom: 12px;
  pointer-events: none;
}

.steps {
  margin: 0;
  padding-left: 22px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.steps li::marker {
  font-weight: 700;
}

.steps li > * + * {
  margin-top: 6px;
}

.host {
  max-width: 320px;
}

.host input {
  font-family: 'Go Mono', ui-monospace, monospace;
  font-size: var(--text-s);
}

.row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.rename input {
  flex: 1;
  min-width: 160px;
  width: auto;
}

.danger-zone {
  margin-top: 18px;
}

.confirm p {
  margin-bottom: 10px;
}

.error {
  margin-top: 10px;
}
</style>
