<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type Device } from '../api'
import Icon from './Icon.vue'

const props = defineProps<{ existing: string[] }>()
const emit = defineEmits<{ close: []; created: [d: Device] }>()

const dialog = ref<HTMLDialogElement>()
const name = ref('')
const error = ref('')
const busy = ref(false)

onMounted(() => dialog.value?.showModal())

// The ID is what the display identifies itself with. It is derived from the
// name, so the user never has to think of one.
const id = computed(() => {
  const base =
    name.value
      .toLowerCase()
      .normalize('NFKD')
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 28) || 'display'
  let candidate = base
  for (let n = 2; props.existing.includes(candidate); n++) candidate = `${base}-${n}`
  return candidate
})

async function create() {
  busy.value = true
  error.value = ''
  try {
    emit('created', await api.createDevice(id.value, name.value.trim()))
    dialog.value?.close()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <dialog ref="dialog" aria-labelledby="add-title" @close="emit('close')">
    <h2 id="add-title">Add a display</h2>
    <form @submit.prevent="create">
      <div>
        <label for="add-name">Where does it hang?</label>
        <input id="add-name" v-model="name" placeholder="Living room" maxlength="60" autocomplete="off" required autofocus />
        <p class="muted small help">
          Its device ID will be <span class="mono ink">{{ id }}</span
          >.
        </p>
      </div>
      <p v-if="error" class="error box" role="alert"><Icon name="warn" :size="18" /> <span>{{ error }}</span></p>
      <div class="actions">
        <button type="submit" class="primary" :disabled="busy || !name.trim()">Add display</button>
        <button type="button" @click="dialog?.close()">Cancel</button>
      </div>
    </form>
  </dialog>
</template>

<style scoped>
h2 {
  font-size: var(--t25);
}

form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 16px;
}

.help {
  margin-top: 6px;
}

.ink {
  color: var(--ink);
}

.error.box svg {
  flex: none;
  margin-top: 1px;
}

.actions {
  display: flex;
  gap: 8px;
}
</style>
