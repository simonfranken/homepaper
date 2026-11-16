<script setup lang="ts">
// Home Assistant connection (D-013). The server tests the connection before
// it saves and never sends the token back, so the token field is write-only.
import { computed, onMounted, ref } from 'vue'
import { api, type HaSettings } from '../api'
import Icon from './Icon.vue'

const emit = defineEmits<{ close: []; changed: [] }>()

const dialog = ref<HTMLDialogElement>()
const settings = ref<HaSettings>()
const url = ref('')
const token = ref('')
const busy = ref<'save' | 'remove'>()
const error = ref('')
const done = ref('')

onMounted(async () => {
  dialog.value?.showModal()
  try {
    settings.value = await api.haSettings()
    url.value = settings.value.url
  } catch (e) {
    error.value = (e as Error).message
  }
})

const status = computed(() => {
  const st = settings.value
  if (!st) return undefined
  if (st.origin === 'none')
    return { cls: 'demo', text: 'Showing demo data. Nothing is connected yet.' }
  if (st.status.connected) return { cls: 'ok', text: `Connected · ${st.status.entities} entities` }
  return { cls: 'bad', text: st.status.error ? `Not connected: ${st.status.error}` : 'Connecting…' }
})

async function save() {
  busy.value = 'save'
  error.value = done.value = ''
  try {
    settings.value = await api.saveHaSettings(url.value.trim(), token.value.trim())
    url.value = settings.value.url
    token.value = ''
    done.value = 'Connected. Displays switch over now; no restart needed.'
    emit('changed')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = undefined
  }
}

async function remove() {
  busy.value = 'remove'
  error.value = done.value = ''
  try {
    settings.value = await api.saveHaSettings('', '')
    url.value = settings.value.url
    token.value = ''
    emit('changed')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = undefined
  }
}
</script>

<template>
  <dialog ref="dialog" aria-labelledby="ha-title" @close="emit('close')">
    <h2 id="ha-title">Home Assistant</h2>
    <p v-if="status" class="status" :class="{ bad: status.cls === 'bad' }">
      <span class="dot" :class="status.cls"></span> {{ status.text }}
    </p>
    <p v-if="settings?.origin === 'env'" class="muted small env">
      This connection comes from <span class="mono ink">HA_URL</span> and <span class="mono ink">HA_TOKEN</span> in the
      server's environment. Saving here takes over from it.
    </p>

    <form @submit.prevent="save">
      <div>
        <label for="ha-url">Address</label>
        <input
          id="ha-url"
          v-model="url"
          class="mono-input"
          placeholder="http://homeassistant.local:8123"
          autocomplete="off"
          spellcheck="false"
          required
        />
      </div>
      <div>
        <label for="ha-token">Long-lived access token</label>
        <input
          id="ha-token"
          v-model="token"
          type="password"
          autocomplete="off"
          :placeholder="settings?.has_token ? 'Saved. Leave empty to keep it' : ''"
          :required="!settings?.has_token"
        />
        <p class="muted small help">
          Create one in Home Assistant: your profile, then Security, then Long-lived access tokens. It stays on the
          homepaper server and is never shown again.
        </p>
      </div>

      <p v-if="error" class="error box" role="alert"><Icon name="warn" :size="18" /> <span>{{ error }}</span></p>
      <p v-if="done" class="ok-box" role="status"><Icon name="check" :size="18" :stroke="3" /> <span>{{ done }}</span></p>

      <div class="actions">
        <button type="submit" class="primary" :disabled="!!busy">
          <span v-if="busy === 'save'" class="spin"></span>
          {{ busy === 'save' ? 'Testing the connection…' : 'Test and save' }}
        </button>
        <button type="button" :disabled="!!busy" @click="dialog?.close()">Close</button>
        <button v-if="settings?.origin === 'ui'" type="button" class="danger quiet remove" :disabled="!!busy" @click="remove">
          Remove connection
        </button>
      </div>
    </form>
  </dialog>
</template>

<style scoped>
h2 {
  font-size: var(--t25);
}

.status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.status.bad {
  color: var(--bad);
  font-weight: 500;
}

.env {
  margin-top: 6px;
}

.ink {
  color: var(--ink);
}

.mono-input {
  font-family: 'Go Mono', ui-monospace, monospace;
}

form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 18px;
}

.help {
  margin-top: 6px;
}

.error.box svg,
.ok-box svg {
  flex: none;
  margin-top: 1px;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.remove {
  margin-left: auto;
}
</style>
