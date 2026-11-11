<script setup lang="ts">
// Home Assistant connection (D-013). The server tests the connection before
// it saves and never sends the token back, so the token field is write-only.
import { computed, onMounted, ref } from 'vue'
import { api, type HaSettings } from '../api'

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
    return { cls: '', text: 'Not connected. homepaper shows demo entities until you connect your Home Assistant.' }
  if (st.status.connected) return { cls: 'ok', text: `Connected. ${st.status.entities} entities available.` }
  return { cls: 'bad', text: st.status.error ? `Not connected: ${st.status.error}` : 'Connecting…' }
})

async function save() {
  busy.value = 'save'
  error.value = done.value = ''
  try {
    settings.value = await api.saveHaSettings(url.value.trim(), token.value.trim())
    url.value = settings.value.url
    token.value = ''
    done.value = 'Connected and saved.'
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
    <p v-if="status" class="status"><span class="dot" :class="status.cls"></span> {{ status.text }}</p>
    <p v-if="settings?.origin === 'env'" class="muted small">
      This connection comes from HA_URL and HA_TOKEN in the server's environment. Saving here takes over from it.
    </p>

    <form @submit.prevent="save">
      <div>
        <label for="ha-url">Address</label>
        <input
          id="ha-url"
          v-model="url"
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

      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="done" class="done small" role="status">{{ done }}</p>

      <div class="actions">
        <button type="submit" class="primary" :disabled="!!busy">
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
.status {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 10px 0 4px;
}

.status .dot {
  transform: translateY(-1px);
}

form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 18px;
}

.help {
  margin-top: 5px;
}

.done {
  color: var(--ok);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}

.remove {
  margin-left: auto;
}
</style>
