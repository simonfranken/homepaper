// The phone layout (D-022, docs/design.md "Phones"). It changes behaviour,
// not only the look (no dragging on the preview, sheets instead of the
// side inspector), so it lives here and not only in CSS. 700 px matches the
// breakpoint in App.vue.
import { ref } from 'vue'

const query = typeof matchMedia === 'function' ? matchMedia('(max-width: 700px)') : undefined

export const phone = ref(query?.matches ?? false)
query?.addEventListener('change', (e) => (phone.value = e.matches))
