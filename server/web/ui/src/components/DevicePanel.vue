<script setup lang="ts">
import { computed, ref } from 'vue'
import { api, deviceUrl, type Device } from '../api'

const props = defineProps<{ device: Device }>()
const emit = defineEmits<{ updated: [d: Device]; deleted: [id: string] }>()

const showToken = ref(false)
const copied = ref('')
const s = computed(() => props.device.status)

async function copy(label: string, text: string) {
  await navigator.clipboard.writeText(text)
  copied.value = label
  setTimeout(() => (copied.value = ''), 1500)
}

async function resetToken() {
  if (!confirm('Issue a new token? The display disconnects until you enter the new token in its setup page.')) return
  emit('updated', { ...(await api.resetToken(props.device.id)), layout: props.device.layout })
}

async function remove() {
  if (!confirm(`Delete "${props.device.name}" and its layout? This cannot be undone.`)) return
  await api.deleteDevice(props.device.id)
  emit('deleted', props.device.id)
}

function ago(iso?: string) {
  if (!iso || iso.startsWith('0001')) return '–'
  const sec = Math.round((Date.now() - new Date(iso).getTime()) / 1000)
  if (sec < 60) return `${sec}s ago`
  if (sec < 3600) return `${Math.round(sec / 60)} min ago`
  return new Date(iso).toLocaleString()
}

function duration(sec?: number) {
  if (!sec) return '–'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h ? `${h}h ${m}m` : `${m}m ${sec % 60}s`
}

const statusText = computed(() => {
  if (!s.value.connected) return 'Offline'
  if (!s.value.last_ack_seq) return 'Connected, waiting for first ack'
  return s.value.in_sync ? 'Online, in sync' : 'Online, out of sync'
})
const statusClass = computed(() => (!s.value.connected ? 'bad' : s.value.in_sync ? 'ok' : 'warn'))
</script>

<template>
  <section class="panel">
    <div class="card">
      <h3>Setup</h3>
      <p class="muted intro">
        On first start (or after holding BOOT for 3 s while it runs) the display opens a Wi-Fi network
        <b>homepaper-setup</b>. Join it, open <span class="mono">http://192.168.71.1</span> and enter:
      </p>
      <dl>
        <dt>Server URL</dt>
        <dd>
          <span class="mono">{{ deviceUrl() }}</span>
          <button @click="copy('url', deviceUrl())">{{ copied === 'url' ? 'Copied' : 'Copy' }}</button>
        </dd>
        <dt>Device ID</dt>
        <dd>
          <span class="mono">{{ device.id }}</span>
          <button @click="copy('id', device.id)">{{ copied === 'id' ? 'Copied' : 'Copy' }}</button>
        </dd>
        <dt>Token</dt>
        <dd>
          <span class="mono">{{ showToken ? device.token : '••••••••••••••••' }}</span>
          <button @click="showToken = !showToken">{{ showToken ? 'Hide' : 'Show' }}</button>
          <button @click="copy('token', device.token)">{{ copied === 'token' ? 'Copied' : 'Copy' }}</button>
        </dd>
      </dl>
      <p class="muted note">
        The server URL must be reachable from the display's Wi-Fi. If this page is open via <i>localhost</i>, use the
        server's LAN address instead.
      </p>
    </div>

    <div class="card">
      <h3>Connection</h3>
      <p class="status"><span class="dot" :class="statusClass"></span> {{ statusText }}</p>
      <dl class="stats">
        <dt>Firmware</dt>
        <dd>{{ s.firmware || '–' }}</dd>
        <dt>Panel</dt>
        <dd>{{ s.panel || '–' }}</dd>
        <dt>Address</dt>
        <dd>{{ s.remote || '–' }}</dd>
        <dt>{{ s.connected ? 'Connected' : 'Last seen' }}</dt>
        <dd>{{ s.connected ? ago(s.connected_at) : ago(device.last_seen) }}</dd>
        <dt>Frames</dt>
        <dd>{{ s.frames_sent }} sent ({{ (s.bytes_sent / 1024).toFixed(1) }} KB), last {{ s.last_kind || '–' }} #{{ s.last_seq }}</dd>
        <dt>Last ack</dt>
        <dd>#{{ s.last_ack_seq }} · {{ ago(s.last_ack_at) }}</dd>
        <dt>Wi-Fi signal</dt>
        <dd>{{ s.rssi ? s.rssi + ' dBm' : '–' }}</dd>
        <dt>Free heap</dt>
        <dd>{{ s.free_heap ? (s.free_heap / 1024).toFixed(0) + ' KB' : '–' }}</dd>
        <dt>Uptime</dt>
        <dd>{{ duration(s.uptime_s) }}</dd>
      </dl>
      <p v-if="s.last_error" class="error">{{ s.last_error }}</p>
    </div>

    <div class="card actions">
      <h3>Manage</h3>
      <button @click="resetToken">New token</button>
      <button class="danger" @click="remove">Delete display</button>
    </div>
  </section>
</template>

<style scoped>
.panel {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 16px;
  align-items: start;
}

h3 {
  font-size: 17px;
  margin-bottom: 10px;
}

.intro,
.note {
  font-size: 13px;
  margin: 0 0 10px;
}

dl {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 12px;
  margin: 0 0 10px;
  font-size: 14px;
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

dd button {
  font-size: 12px;
  padding: 2px 8px;
}

.status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 10px;
}

.actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}
</style>
