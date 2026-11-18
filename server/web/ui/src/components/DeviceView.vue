<script setup lang="ts">
// Everything about one display that isn't its layout: getting it connected,
// how the connection is doing, and renaming or removing it.
import { computed, ref, watch } from 'vue'
import { ago, api, deviceUrl, hasConnected, isLoopback, setupLink, SETUP_PORTAL, SETUP_SSID, type Device } from '../api'
import Icon from './Icon.vue'
import DisplayArea from './DisplayArea.vue'

const props = defineProps<{ device: Device }>()
const emit = defineEmits<{ updated: [d: Device]; deleted: [id: string]; openLayout: [] }>()

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
          cls: '',
          title: 'Offline',
          text: `Last seen ${ago(props.device.last_seen) || 'a while ago'}. The display keeps showing its last frame and reconnects by itself once it has power and Wi-Fi.`,
        }
      : {
          cls: 'never',
          title: 'Not connected yet',
          text: "The display hasn't reached this server yet. Its layout is ready; it appears the moment the display connects.",
        }
  }
  if (!s.value.last_ack_seq)
    return { cls: 'warn', title: 'Connected', text: 'Sending the first frame. A full refresh takes a few seconds.' }
  if (s.value.in_sync)
    return { cls: 'ok', title: 'Online · in sync', text: `Showing the current layout. Last update ${ago(s.value.last_ack_at)}.` }
  return {
    cls: 'warn',
    title: 'Online · catching up',
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
    <div class="main">
      <section class="card setup" aria-labelledby="setup-h">
        <details :open="!everConnected">
          <summary>
            <h2 id="setup-h">{{ everConnected ? 'Set it up again' : 'Connect the display' }}</h2>
            <p class="muted">Four steps, about two minutes. You need a phone or laptop with Wi-Fi.</p>
          </summary>
          <ol class="steps">
            <li>
              <span class="num" aria-hidden="true">1</span>
              <div>
                <p class="lead">Power the display. A new display opens the Wi-Fi network <b>{{ SETUP_SSID }}</b>.</p>
                <p class="small muted">One that was set up before does so after you hold its BOOT button for 3 seconds while it runs.</p>
              </div>
            </li>
            <li>
              <span class="num" aria-hidden="true">2</span>
              <div>
                <label for="host" class="lead plain">Check the address the display should reach this server at.</label>
                <input
                  id="host"
                  v-model="host"
                  class="host"
                  :class="{ bad: loopback }"
                  spellcheck="false"
                  autocomplete="off"
                  :placeholder="pageHost"
                />
                <p v-if="loopback" class="notice">
                  <Icon name="warn" :size="18" />
                  <span>
                    <b>{{ hostValue }}</b> only works on this computer. Enter the server's address on your network instead,
                    for example 192.168.1.10:8080.
                  </span>
                </p>
                <p v-else class="small muted">The display connects to <span class="mono ink">{{ serverUrl }}</span></p>
              </div>
            </li>
            <li>
              <span class="num" aria-hidden="true">3</span>
              <div>
                <p class="lead">
                  Join <b>{{ SETUP_SSID }}</b> with this computer or phone, then open the setup page. It arrives filled in;
                  you only choose your Wi-Fi.
                </p>
                <p class="small muted">This page stops loading while you're on that network. That's expected.</p>
                <div v-if="!loopback" class="row">
                  <a class="button primary" :href="link" target="_blank" rel="noopener">
                    Open the setup page
                    <Icon name="external" :size="14" :stroke="3" />
                  </a>
                  <button @click="copy('link', link)">
                    <Icon :name="copied === 'link' ? 'check' : 'copy'" :size="14" />
                    {{ copied === 'link' ? 'Copied' : 'Copy its link' }}
                  </button>
                </div>
                <div v-else class="row">
                  <button class="primary" disabled>Open the setup page</button>
                  <span class="small muted">Enter a network address in step 2 first.</span>
                </div>
              </div>
            </li>
            <li>
              <span class="num" aria-hidden="true">4</span>
              <div>
                <p class="lead">Save there. The display restarts, joins your Wi-Fi and shows up here as online.</p>
              </div>
            </li>
          </ol>

          <details class="manual">
            <summary>
              <Icon name="chevron" :size="14" :stroke="3" />
              Enter the values by hand
            </summary>
            <div class="manual-body">
              <p class="small muted">Open <span class="mono ink">{{ SETUP_PORTAL }}</span> on the setup network and type these in.</p>
              <dl class="values">
                <dt>Server URL</dt>
                <dd>
                  <span class="mono ink">{{ serverUrl }}</span>
                  <button class="small" @click="copy('url', serverUrl)">{{ copied === 'url' ? 'Copied' : 'Copy' }}</button>
                </dd>
                <dt>Device ID</dt>
                <dd>
                  <span class="mono ink">{{ device.id }}</span>
                  <button class="small" @click="copy('id', device.id)">{{ copied === 'id' ? 'Copied' : 'Copy' }}</button>
                </dd>
                <dt>Token</dt>
                <dd>
                  <span class="mono ink">{{ showToken ? device.token : '•'.repeat(16) }}</span>
                  <button class="small" @click="showToken = !showToken">{{ showToken ? 'Hide' : 'Show' }}</button>
                  <button class="small" @click="copy('token', device.token)">{{ copied === 'token' ? 'Copied' : 'Copy' }}</button>
                </dd>
              </dl>
            </div>
          </details>
        </details>
      </section>
      <DisplayArea
        :device="device"
        :class="{ first: everConnected }"
        @updated="(d) => emit('updated', d)"
        @layout="emit('openLayout')"
      />
    </div>

    <aside class="side">
      <section class="card status" aria-labelledby="status-h">
        <h2 id="status-h" class="cap">Status</h2>
        <p class="state"><span class="dot big" :class="state.cls"></span> {{ state.title }}</p>
        <p class="muted">{{ state.text }}</p>
        <p v-if="s.last_error && !s.in_sync" class="error">{{ s.last_error }}</p>

        <details v-if="everConnected" class="details">
          <summary>Details</summary>
          <dl>
            <dt>Firmware</dt>
            <dd class="mono">{{ s.firmware || '–' }}</dd>
            <dt>Panel</dt>
            <dd class="mono">{{ s.panel || `${device.panel.model} ${device.panel.width}×${device.panel.height}` }}</dd>
            <dt>Drawn area</dt>
            <dd class="mono">
              {{ device.area.w }}×{{ device.area.h }} at {{ device.area.x }}, {{ device.area.y }} · {{ device.grid.cols }}×{{
                device.grid.rows
              }}
              cells
            </dd>
            <dt>Address</dt>
            <dd class="mono">{{ s.remote || '–' }}</dd>
            <dt>Wi-Fi signal</dt>
            <dd>{{ signal(s.rssi) }}</dd>
            <dt>Running for</dt>
            <dd>{{ duration(s.uptime_s) }}</dd>
            <dt>Free memory</dt>
            <dd class="mono">{{ s.free_heap ? Math.round(s.free_heap / 1024) + ' KB' : '–' }}</dd>
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

      <section class="card manage" aria-labelledby="manage-h">
        <div class="manage-top">
          <h2 id="manage-h" class="cap">Manage</h2>
          <form class="rename" @submit.prevent="rename">
            <label for="dev-name">Name</label>
            <div class="row nowrap">
              <input id="dev-name" v-model="name" maxlength="60" autocomplete="off" />
              <button type="submit" :disabled="!name.trim() || name.trim() === device.name">Rename</button>
            </div>
          </form>
        </div>

        <div class="danger-zone">
          <div v-if="confirming === 'token'" class="confirm">
            <p>The display disconnects until you run its setup again with the new token.</p>
            <div class="row">
              <button class="danger" @click="resetToken">Replace token</button>
              <button @click="confirming = undefined">Cancel</button>
            </div>
          </div>
          <div v-else-if="confirming === 'delete'" class="confirm bad">
            <p>This removes “{{ device.name }}” and its layout for good.</p>
            <div class="row">
              <button class="danger solid" @click="remove">Delete display</button>
              <button @click="confirming = undefined">Cancel</button>
            </div>
          </div>
          <template v-else>
            <div class="row">
              <button class="small" title="For when the token may have leaked" @click="confirming = 'token'">Replace token…</button>
              <button class="small danger" @click="confirming = 'delete'">Delete display…</button>
            </div>
            <p class="small muted">Replace the token if it may have leaked. The display then needs its setup again.</p>
          </template>
          <p v-if="error" class="error">{{ error }}</p>
        </div>
      </section>
    </aside>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-wrap: wrap;
  gap: 28px;
  align-items: flex-start;
}

.main {
  flex: 999 1 560px;
  min-width: 0;
  max-width: 860px;
  display: flex;
  flex-direction: column;
  gap: 28px;
}

/* Once the display is set up, its display area matters more than the steps. */
.main > .first {
  order: -1;
}

.setup > details > summary {
  display: block;
  padding: 20px 24px 16px;
  list-style: none;
  cursor: pointer;
}

.setup > details > summary::-webkit-details-marker {
  display: none;
}

.setup > details[open] > summary {
  border-bottom: 2px dotted var(--ink);
}

.setup > details:not([open]) > summary h2::after {
  content: ' ▸';
}

.setup summary p {
  margin-top: 4px;
}

.steps {
  list-style: none;
  padding: 0;
}

.steps > li {
  display: flex;
  gap: 16px;
  padding: 20px 24px;
}

.steps > li + li {
  border-top: 2px dotted var(--ink);
}

.steps > li > div {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 5px;
}

.num {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: none;
  border-radius: 50%;
  background: var(--ink);
  color: var(--on-ink);
  font-size: var(--t17);
  font-weight: 700;
}

.lead {
  font-size: var(--t17);
}

label.plain {
  margin: 0;
  font-weight: 400;
}

.host {
  max-width: 360px;
  font-family: 'Go Mono', ui-monospace, monospace;
}

.host.bad {
  border-color: var(--bad);
}

.notice {
  display: flex;
  gap: 10px;
}

.notice svg {
  flex: none;
  margin-top: 1px;
}

.ink {
  color: var(--ink);
}

.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.row.nowrap {
  flex-wrap: nowrap;
}

.manual {
  border-top: 2px solid var(--ink);
}

.manual > summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 24px;
  font-weight: 700;
  list-style: none;
  cursor: pointer;
}

.manual > summary::-webkit-details-marker {
  display: none;
}

.manual:not([open]) > summary svg {
  transform: rotate(-90deg);
}

.manual-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 0 24px 22px;
}

.values {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  border: 2px solid var(--ink);
  border-radius: var(--radius);
  overflow: hidden;
}

.values dt {
  padding: 10px 14px;
  font-weight: 700;
}

.values dd {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
}

.values dd .mono {
  flex: 1;
  min-width: 0;
}

.values dt:not(:last-of-type),
.values dd:not(:last-of-type) {
  border-bottom: 2px dotted var(--ink);
}

.side {
  flex: 1 1 340px;
  min-width: 0;
  max-width: 460px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.status {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 18px 18px 20px;
}

.state {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: var(--t20);
  font-weight: 700;
}

.details summary {
  padding: 4px 0;
  font-weight: 700;
  cursor: pointer;
}

.details dl {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 6px 16px;
  margin-top: 8px;
  font-size: var(--t12);
}

.details dt {
  color: var(--muted);
}

.manage-top {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px 18px 16px;
}

.rename input {
  flex: 1;
  min-width: 0;
}

.danger-zone {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 18px 18px;
  border-top: 2px dotted var(--ink);
}

.confirm {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>
