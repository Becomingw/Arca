<template>
  <div
    class="arca-feature-content arca-operations"
    role="group"
    :aria-label="t('site.home.operations.illustration')"
    :data-motion="running ? 'running' : 'paused'"
  >
    <h2
      id="operations-title"
      class="arca-feature-heading"
    >
      <span class="operations-title-part">{{ t('site.home.operations.titleLead') }}</span><span class="operations-title-part">{{ t('site.home.operations.titleTail') }}</span>
    </h2>
    <p class="arca-feature-description">
      {{ t('site.home.operations.description') }}
    </p>
    <div class="operations-workflow">
      <fieldset
        class="operations-timeline"
        :aria-label="t('site.home.operations.workflow')"
      >
        <legend class="sr-only">
          {{ t('site.home.operations.workflow') }}
        </legend>
        <label
          v-for="(step, index) in operationSteps"
          :key="step.id"
          class="operations-step"
          :class="{ 'is-active': selected === index, 'is-complete': selected > index }"
        >
          <input
            type="radio"
            name="home-operations"
            :value="index"
            :checked="selected === index"
            :aria-label="t(step.title)"
            @change="selectStep(index)"
          >
          <span
            class="operations-node"
            aria-hidden="true"
          >
            <Check v-if="selected > index" />
            <component
              :is="stepIcons[step.icon]"
              v-else
            />
          </span>
          <span class="operations-step-copy">
            <span class="operations-step-title">{{ t(step.title) }}</span>
            <span class="operations-step-description">{{ t(step.description) }}</span>
          </span>
        </label>
      </fieldset>
      <div class="operations-ledger">
        <div class="operations-ledger-heading">
          <h3>{{ t('site.home.operations.ledger') }}</h3>
          <button
            class="arca-motion-toggle"
            :title="t(paused ? 'site.home.motion.play' : 'site.home.motion.pause')"
            :aria-label="t(paused ? 'site.home.motion.play' : 'site.home.motion.pause')"
            :aria-pressed="paused"
            :disabled="reducedMotion"
            @click="toggle"
          >
            <Play
              v-if="paused"
              aria-hidden="true"
            /><Pause
              v-else
              aria-hidden="true"
            />
          </button>
        </div>
        <ol
          class="operations-tool-list"
          :aria-label="t('site.home.operations.tools')"
        >
          <li
            v-for="(tool, index) in current.tools"
            :key="`${current.id}-${index}`"
            :data-state="toolState(index)"
          >
            <span
              class="operations-tool-icon"
              aria-hidden="true"
            >
              <Check v-if="index < beat" />
              <LoaderCircle v-else-if="index === beat" />
              <Circle v-else />
            </span>
            <span class="operations-tool-name">{{ tool.name }}</span>
            <span class="operations-tool-status">{{ t(statusKeys[toolState(index)]) }}</span>
            <span class="operations-tool-label">{{ t(tool.label) }}</span>
          </li>
        </ol>
        <div
          class="operations-result"
          :aria-live="running ? 'off' : 'polite'"
          aria-atomic="true"
        >
          <p class="operations-result-title">
            <span aria-hidden="true" />{{ t(current.title) }}
          </p>
          <p>{{ t(current.result) }}</p>
        </div>
      </div>
    </div>
    <ul class="arca-feature-points operations-points">
      <li><ShieldCheck aria-hidden="true" /><span>{{ t('site.home.operations.identity') }}</span></li>
      <li><UsersRound aria-hidden="true" /><span>{{ t('site.home.operations.permissions') }}</span></li>
      <li><FileClock aria-hidden="true" /><span>{{ t('site.home.operations.audit') }}</span></li>
    </ul>
    <div class="operations-actions">
      <a :href="feedbackPath" class="operations-feedback-link">
        {{ t('site.home.operations.feedback') }}<ArrowRight aria-hidden="true" />
      </a>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRight, Check, CheckCircle2, Circle, FileClock, LoaderCircle, MessageSquareText, Pause, Play, Search, ShieldCheck, UsersRound, Wrench } from 'lucide-vue-next'
import { useI18n } from './copy'
import { operationSteps, OPERATIONS_BEAT_MS } from './operations-example'
import { useFeatureMotion } from './useFeatureMotion'

const props = withDefaults(defineProps<{ active?: boolean, feedbackPath: string }>(), { active: false })
const { t } = useI18n()
const selected = ref(1)
const beat = ref(1)
const current = computed(() => operationSteps[selected.value] ?? operationSteps[0])
const stepIcons = { message: MessageSquareText, search: Search, wrench: Wrench, check: CheckCircle2 }
const statusKeys = { complete: 'site.home.operations.complete', running: 'site.home.operations.running', pending: 'site.home.operations.pending' } as const
const { paused, running, reducedMotion, toggle } = useFeatureMotion(() => props.active)
watch(running, (active, _, onCleanup) => {
  if (!active) return
  const timer = setInterval(() => {
    if (beat.value < current.value.tools.length) beat.value++
    else {
      selected.value = (selected.value + 1) % operationSteps.length
      beat.value = 0
    }
  }, OPERATIONS_BEAT_MS)
  onCleanup(() => clearInterval(timer))
}, { immediate: true })
function selectStep(index: number) {
  selected.value = index
  beat.value = 0
  paused.value = true
}
function toolState(index: number) { return index < beat.value ? 'complete' : index === beat.value ? 'running' : 'pending' }
</script>

<style scoped>
.operations-title-part { display: inline-block; max-width: 100%; }
.operations-workflow { display: grid; grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr); gap: 72px; margin: 56px auto 0; max-width: 960px; }
.operations-timeline { position: relative; display: grid; gap: 12px; min-width: 0; border: 0; padding: 0; }
.operations-timeline::before { content: ''; position: absolute; left: 14px; top: 18px; bottom: 50px; width: 1px; background: var(--home-border); }
.operations-step { position: relative; display: flex; align-items: flex-start; gap: 26px; min-height: 84px; padding: 6px 0 12px; cursor: pointer; }
.operations-step input { position: absolute; inset: 0; width: 100%; height: 100%; opacity: 0; cursor: pointer; z-index: 1; margin: 0; }
.operations-node { position: relative; display: grid; place-items: center; flex-shrink: 0; width: 29px; height: 29px; border: 1px solid var(--home-border); border-radius: 50%; background: var(--home-background); color: var(--home-muted); transition: color 200ms, border-color 200ms, background 200ms; }
.operations-node svg { width: 16px; height: 16px; stroke-width: 1.7; }
.operations-step-copy { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.operations-step-title { font-family: var(--serif); font-size: 24px; line-height: 32px; }
.operations-step-description { font-family: var(--serif); font-size: 15px; line-height: 24px; color: var(--home-muted); }
.operations-step.is-active .operations-step-title { color: var(--home-accent); }
.operations-step.is-active .operations-node { border-color: var(--home-accent); color: var(--home-accent); box-shadow: 0 0 0 3px var(--home-hover); }
.operations-step.is-complete .operations-node { background: #728f89; border-color: #728f89; color: white; }
.operations-step input:focus-visible ~ .operations-step-copy { outline: 2px solid var(--home-accent); outline-offset: 4px; border-radius: 4px; }
.operations-ledger { min-width: 0; }
.operations-ledger-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding-bottom: 16px; border-bottom: 1px solid var(--home-border); }
.operations-ledger-heading h3 { font-family: var(--serif); font-size: 20px; line-height: 30px; font-weight: 500; }
.operations-tool-list { list-style: none; padding: 0; margin: 14px 0 0; }
.operations-tool-list li { display: grid; grid-template-columns: 20px minmax(0, 1fr) 54px; align-items: center; column-gap: 14px; min-height: 68px; padding: 12px 0; border-bottom: 1px solid var(--home-border); }
.operations-tool-icon { grid-row: 1 / 3; display: grid; place-items: center; color: var(--home-muted); }
.operations-tool-icon svg { width: 18px; height: 18px; stroke-width: 1.6; }
.operations-tool-name { min-width: 0; font-family: var(--font-mono, monospace); font-size: 13px; line-height: 20px; overflow-wrap: anywhere; }
.operations-tool-status { font-family: var(--sans-serif); font-size: 11px; line-height: 18px; text-align: right; color: var(--home-muted); }
.operations-tool-label { grid-column: 2; font-family: var(--serif); font-size: 14px; line-height: 20px; color: var(--home-muted); }
[data-state='complete'] .operations-tool-icon, [data-state='complete'] .operations-tool-status { color: #728f89; }
[data-state='running'] .operations-tool-icon, [data-state='running'] .operations-tool-status { color: var(--home-accent); }
[data-state='running'] .operations-tool-icon svg { animation: operations-spin 2s linear infinite paused; }
[data-motion='running'] [data-state='running'] .operations-tool-icon svg { animation-play-state: running; }
.operations-result { margin-top: 26px; min-height: 120px; font-family: var(--serif); font-size: 19px; line-height: 30px; }
.operations-result-title { display: flex; align-items: center; gap: 12px; margin-bottom: 8px; color: var(--home-accent); font-size: 20px; }
.operations-result-title > span { width: 8px; height: 8px; border-radius: 50%; background: currentColor; flex-shrink: 0; }
.operations-points { margin-top: 38px; }
.operations-actions { margin-top: 28px; text-align: center; }
.operations-feedback-link { display: inline-flex; align-items: center; gap: 10px; padding: 4px 0; border-bottom: 1px solid var(--home-accent); font-family: var(--serif); font-size: 16px; line-height: 24px; color: var(--home-accent); }
.operations-feedback-link svg { width: 18px; height: 18px; stroke-width: 1.5; }
.operations-feedback-link:focus-visible { outline: 2px solid var(--home-accent); outline-offset: 4px; }
@keyframes operations-spin { to { transform: rotate(360deg); } }
@media (max-width: 1023px) { .operations-workflow { gap: 36px; } .operations-step { gap: 18px; } .operations-step-title { font-size: 21px; } }
@media (max-width: 700px) {
  .operations-workflow { grid-template-columns: minmax(0, 1fr); gap: 24px; margin-top: 30px; }
  .operations-timeline { gap: 4px; }
  .operations-timeline::before { bottom: 36px; }
  .operations-step { min-height: 68px; gap: 16px; padding-bottom: 8px; }
  .operations-step-title { font-size: 19px; line-height: 28px; }
  .operations-step-description { font-size: 13px; line-height: 21px; }
  .operations-ledger-heading { padding-bottom: 10px; }
  .operations-ledger-heading h3 { font-size: 18px; }
  .operations-tool-list { margin-top: 6px; }
  .operations-tool-list li { min-height: 84px; column-gap: 10px; grid-template-columns: 18px minmax(0, 1fr) 44px; }
  .operations-tool-name { font-size: 12px; line-height: 18px; }
  .operations-tool-status { font-size: 9px; }
  .operations-tool-label { font-size: 12px; }
  .operations-result { margin-top: 20px; min-height: 144px; font-size: 17px; line-height: 26px; }
  .operations-result-title { font-size: 18px; }
  .operations-points { margin-top: 24px; }
  .operations-actions { margin-top: 22px; }
}
@media (prefers-reduced-motion: reduce) { *, *::before { animation: none !important; transition: none !important; } }
</style>
