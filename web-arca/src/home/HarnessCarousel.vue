<template>
  <div
    class="harness-carousel"
    role="region"
    aria-roledescription="carousel"
    :aria-label="t('site.home.harness.label')"
    :data-paused="!rotating"
    @keydown.left.prevent="select(index - 1)"
    @keydown.right.prevent="select(index + 1)"
  >
    <div
      class="harness-stage"
      @pointerenter="hovered = true"
      @pointerleave="hovered = false"
    >
      <button
        class="harness-neighbor"
        :title="t('site.home.harness.show', { name: previous.name })"
        :aria-label="t('site.home.harness.show', { name: previous.name })"
        @click="select(index - 1)"
      >
        <img
          :src="previous.logo"
          alt=""
          :class="{ monochrome: previous.monochrome }"
          width="32"
          height="32"
        >
        <span>{{ previous.name }}</span>
      </button>

      <div class="harness-spotlight">
        <Transition
          name="harness-swap"
          mode="out-in"
        >
          <div
            :key="current.id"
            class="harness-current"
            :data-harness="current.id"
            :aria-live="paused ? 'polite' : 'off'"
          >
            <div
              class="harness-mark"
              :class="`motion-${current.id}`"
              aria-hidden="true"
            >
              <img
                :src="current.logo"
                alt=""
                :class="{ monochrome: current.monochrome }"
                width="76"
                height="76"
              >
            </div>
            <h2>{{ current.name }}</h2>
            <p>{{ current.protocol }}</p>
          </div>
        </Transition>
      </div>

      <button
        class="harness-neighbor"
        :title="t('site.home.harness.show', { name: next.name })"
        :aria-label="t('site.home.harness.show', { name: next.name })"
        @click="select(index + 1)"
      >
        <img
          :src="next.logo"
          alt=""
          :class="{ monochrome: next.monochrome }"
          width="32"
          height="32"
        >
        <span>{{ next.name }}</span>
      </button>
    </div>

    <div class="harness-controls">
      <button
        class="harness-control"
        :title="t('site.home.harness.previous')"
        :aria-label="t('site.home.harness.previous')"
        @click="select(index - 1)"
      >
        <ChevronLeft aria-hidden="true" />
      </button>
      <button
        class="harness-control"
        :title="t(paused ? 'site.home.harness.play' : 'site.home.harness.pause')"
        :aria-label="t(paused ? 'site.home.harness.play' : 'site.home.harness.pause')"
        :aria-pressed="paused"
        :disabled="reducedMotion"
        @click="paused = !paused"
      >
        <Play
          v-if="paused"
          aria-hidden="true"
        />
        <Pause
          v-else
          aria-hidden="true"
        />
      </button>
      <button
        class="harness-control"
        :title="t('site.home.harness.next')"
        :aria-label="t('site.home.harness.next')"
        @click="select(index + 1)"
      >
        <ChevronRight aria-hidden="true" />
      </button>
    </div>
    <div class="harness-pagination">
      <button
        v-for="(harness, itemIndex) in harnesses"
        :key="harness.id"
        :aria-label="t('site.home.harness.show', { name: harness.name })"
        :aria-pressed="index === itemIndex"
        :title="harness.name"
        @click="select(itemIndex)"
      >
        <span :class="{ selected: index === itemIndex }" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, Pause, Play } from 'lucide-vue-next'
import { useI18n } from './copy'
import { harnesses, HARNESS_ROTATION_MS, wrapHarnessIndex } from './harnesses'

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: true })
const { t } = useI18n()
const index = ref(1)
const paused = ref(false)
const hovered = ref(false)
const hidden = ref(document.hidden)
const reducedMotion = ref(false)
const current = computed(() => harnesses[index.value] ?? harnesses[0])
const previous = computed(() => harnesses[wrapHarnessIndex(index.value - 1)] ?? harnesses[0])
const next = computed(() => harnesses[wrapHarnessIndex(index.value + 1)] ?? harnesses[0])
const rotating = computed(() => props.active && !paused.value && !hovered.value && !hidden.value && !reducedMotion.value)
let timer: ReturnType<typeof setInterval> | undefined
let motionQuery: MediaQueryList | undefined

function select(nextIndex: number) {
  index.value = wrapHarnessIndex(nextIndex)
  paused.value = true
}

function syncVisibility() {
  hidden.value = document.hidden
}

function syncMotion() {
  reducedMotion.value = motionQuery?.matches ?? false
  if (reducedMotion.value) paused.value = true
}

function syncTimer() {
  clearInterval(timer)
  timer = undefined
  if (rotating.value) {
    timer = setInterval(() => {
      index.value = wrapHarnessIndex(index.value + 1)
    }, HARNESS_ROTATION_MS)
  }
}

watch(rotating, syncTimer)
onMounted(() => {
  motionQuery = window.matchMedia?.('(prefers-reduced-motion: reduce)')
  syncMotion()
  motionQuery?.addEventListener('change', syncMotion)
  document.addEventListener('visibilitychange', syncVisibility)
  syncTimer()
})
onUnmounted(() => {
  clearInterval(timer)
  motionQuery?.removeEventListener('change', syncMotion)
  document.removeEventListener('visibilitychange', syncVisibility)
})
</script>

<style scoped>
.harness-carousel { width: min(100%, 620px); color: var(--home-text); }
.harness-stage { position: relative; display: grid; grid-template-columns: minmax(72px, 1fr) minmax(164px, 220px) minmax(72px, 1fr); align-items: center; min-height: 174px; }
.harness-stage::before, .harness-stage::after { content: ''; position: absolute; top: 63px; width: 13%; height: 1px; background: var(--home-border); }
.harness-stage::before { left: 22%; }
.harness-stage::after { right: 22%; }
.harness-neighbor { position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; min-height: 112px; color: var(--home-muted); opacity: 0.65; transition: opacity 180ms, transform 180ms; }
.harness-neighbor:hover, .harness-neighbor:focus-visible { opacity: 1; transform: translateY(-3px); }
.harness-neighbor img { width: 32px; height: 32px; object-fit: contain; }
.harness-neighbor span { font-family: var(--serif); font-size: 15px; line-height: 20px; text-align: center; overflow-wrap: anywhere; }
.harness-spotlight { position: relative; min-height: 174px; }
.harness-current { display: flex; flex-direction: column; align-items: center; text-align: center; }
.harness-mark { display: grid; place-items: center; width: 96px; height: 96px; transform-origin: center; }
.harness-mark img { width: 76px; height: 76px; object-fit: contain; }
.harness-current h2 { margin-top: 10px; font-family: var(--serif); font-size: 26px; line-height: 34px; font-weight: 500; letter-spacing: 0; }
.harness-current p { margin-top: 4px; font-family: var(--serif); font-size: 14px; line-height: 22px; color: var(--home-muted); white-space: nowrap; }
.harness-controls { display: flex; align-items: center; justify-content: center; gap: 22px; margin-top: 14px; }
.harness-control { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 6px; color: var(--home-muted); transition: color 160ms, background 160ms; }
.harness-control svg { width: 18px; height: 18px; stroke-width: 1.75; }
.harness-control:hover { color: var(--home-accent); background: var(--home-hover); }
.harness-control:disabled { opacity: 0.4; cursor: default; }
button:focus-visible { outline: 2px solid var(--home-accent); outline-offset: 3px; border-radius: 6px; }
.harness-pagination { display: flex; justify-content: center; margin-top: 7px; }
.harness-pagination button { display: grid; place-items: center; width: 26px; height: 26px; }
.harness-pagination span { width: 6px; height: 6px; border-radius: 50%; background: var(--home-muted); opacity: 0.25; transition: opacity 180ms, background 180ms; }
.harness-pagination .selected { background: var(--home-accent); opacity: 1; }
[data-theme="dark"] .monochrome { filter: invert(1); }
.motion-claude { animation: harness-turn 4.8s ease-in-out infinite; }
.motion-codex { animation: harness-orbit 4.8s ease-in-out infinite; }
.motion-gemini { animation: harness-spark 4.8s ease-in-out infinite; }
.motion-opencode { animation: harness-reveal 4.8s ease-in-out infinite; }
.motion-cline { animation: harness-nod 4.8s ease-in-out infinite; }
[data-paused='true'] .harness-mark { animation-play-state: paused; }
.harness-swap-enter-active, .harness-swap-leave-active { transition: opacity 240ms, transform 240ms; }
.harness-swap-enter-from { opacity: 0; transform: translateY(10px); }
.harness-swap-leave-to { opacity: 0; transform: translateY(-10px); }
@keyframes harness-turn { 0%, 100% { transform: rotate(0) scale(1); } 50% { transform: rotate(22deg) scale(1.06); } }
@keyframes harness-orbit { 0%, 100% { transform: rotate(-7deg); } 50% { transform: rotate(7deg); } }
@keyframes harness-spark { 0%, 100% { transform: scale(0.94) rotate(-8deg); } 50% { transform: scale(1.07) rotate(8deg); } }
@keyframes harness-reveal { 0%, 100% { clip-path: inset(0); transform: translateY(0); } 12% { clip-path: inset(0 0 28%); transform: translateY(3px); } 25%, 75% { clip-path: inset(0); transform: translateY(-3px); } }
@keyframes harness-nod { 0%, 65%, 100% { transform: rotate(0); } 78% { transform: rotate(-8deg) translateY(-4px); } 90% { transform: rotate(5deg); } }
@media (max-width: 480px) {
  .harness-stage { grid-template-columns: minmax(58px, 1fr) minmax(150px, 180px) minmax(58px, 1fr); min-height: 156px; }
  .harness-spotlight { min-height: 156px; }
  .harness-mark { width: 80px; height: 80px; }
  .harness-mark img { width: 62px; height: 62px; }
  .harness-neighbor img { width: 26px; height: 26px; }
  .harness-neighbor span { font-size: 12px; line-height: 17px; }
  .harness-current h2 { font-size: 23px; line-height: 30px; }
  .harness-current p { font-size: 11px; line-height: 18px; }
  .harness-stage::before, .harness-stage::after { top: 52px; }
}
@media (max-width: 359px) {
  .harness-neighbor span { font-size: 11px; overflow-wrap: normal; }
}
@media (max-height: 600px) {
  .harness-stage, .harness-spotlight { min-height: 128px; }
  .harness-mark { width: 64px; height: 64px; }
  .harness-mark img { width: 48px; height: 48px; }
  .harness-current h2 { margin-top: 8px; font-size: 21px; line-height: 28px; }
  .harness-current p { margin-top: 0; font-size: 11px; line-height: 18px; }
  .harness-neighbor { min-height: 90px; }
  .harness-stage::before, .harness-stage::after { top: 40px; }
  .harness-controls { margin-top: 4px; }
  .harness-control { height: 32px; }
  .harness-pagination { margin-top: 0; }
  .harness-pagination button { height: 22px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before { animation: none !important; transition: none !important; }
}
</style>
