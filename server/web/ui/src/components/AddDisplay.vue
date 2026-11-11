<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type Device } from '../api'

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
          Its device ID will be <span class="mono">{{ id }}</span
          >.
        </p>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="actions">
        <button type="submit" class="primary" :disabled="busy || !name.trim()">Add display</button>
        <button type="button" @click="dialog?.close()">Cancel</button>
      </div>
    </form>
  </dialog>
</template>

<style scoped>
form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 16px;
}

.help {
  margin-top: 5px;
}

.actions {
  display: flex;
  gap: 8px;
}
</style>
