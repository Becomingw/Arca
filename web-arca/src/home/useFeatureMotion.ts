import { computed, onMounted, onUnmounted, ref } from 'vue'

export function useFeatureMotion(active: () => boolean) {
  const paused = ref(false)
  const visible = ref(!document.hidden)
  const preference = window.matchMedia('(prefers-reduced-motion: reduce)')
  const reducedMotion = ref(preference.matches)
  const running = computed(() => active() && !paused.value && !reducedMotion.value && visible.value)

  function syncVisibility() { visible.value = !document.hidden }
  function syncMotion() { reducedMotion.value = preference.matches }
  function toggle() { paused.value = !paused.value }

  onMounted(() => {
    document.addEventListener('visibilitychange', syncVisibility)
    preference.addEventListener('change', syncMotion)
  })
  onUnmounted(() => {
    document.removeEventListener('visibilitychange', syncVisibility)
    preference.removeEventListener('change', syncMotion)
  })

  return { paused, running, reducedMotion, toggle }
}
