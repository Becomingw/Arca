<template>
  <div
    class="arca-feature-content arca-protocol"
    :data-active="active"
    :data-motion="running ? 'running' : 'paused'"
  >
    <h2
      id="protocol-title"
      class="arca-feature-heading"
    >
      {{ t('site.home.protocol.title') }}
    </h2>
    <p class="arca-feature-description">
      {{ t('site.home.protocol.description') }}
    </p>
    <div class="protocol-flow">
      <fieldset
        class="protocol-clients"
        role="radiogroup"
        :aria-label="t('site.home.protocol.client')"
      >
        <legend>{{ t('site.home.protocol.client') }}</legend>
        <label
          v-for="protocol in protocolExamples"
          :key="protocol.id"
          class="protocol-option"
        >
          <input
            v-model="selected"
            type="radio"
            name="home-protocol"
            :value="protocol.id"
            :aria-label="`${protocol.name} ${protocol.format}`"
          >
          <span class="protocol-option-content">
            <img
              :src="protocol.icon"
              alt=""
              :class="{ 'is-monochrome': protocol.monochrome }"
              width="32"
              height="32"
            >
            <span><span class="protocol-name">{{ protocol.name }}</span><span class="protocol-format">{{ protocol.format }}</span></span>
          </span>
        </label>
      </fieldset>
      <div
        class="protocol-wire wire-in"
        aria-hidden="true"
      >
        <ChevronRight /><span class="protocol-current" />
      </div>
      <div class="protocol-gateway">
        <svg
          :viewBox="ARCA_LOGO_VIEWBOX"
          aria-hidden="true"
        >
          <path
            :d="ARCA_LOGO_PATH"
            fill="currentColor"
            opacity="0.16"
          />
          <path
            :d="ARCA_LOGO_PATH"
            fill="currentColor"
            class="protocol-logo-fill"
          />
          <path
            :d="ARCA_LOGO_PATH"
            fill="none"
            class="protocol-logo-outline"
          />
        </svg>
        <div class="protocol-gateway-brand">
          <span class="protocol-gateway-name">Arca</span>
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
        <span class="protocol-gateway-caption">{{ t('site.home.protocol.gateway') }}</span>
      </div>
      <div
        class="protocol-wire wire-out"
        aria-hidden="true"
      >
        <ChevronRight /><span class="protocol-current" />
      </div>
      <div
        class="protocol-upstream"
        aria-live="polite"
        aria-atomic="true"
      >
        <p class="protocol-column-label">
          {{ t('site.home.protocol.upstream') }}
        </p>
        <img
          :key="current.targetIcon"
          :src="current.targetIcon"
          alt=""
          :class="{ 'is-monochrome': current.targetMonochrome }"
          width="72"
          height="72"
        >
        <span class="protocol-name">{{ current.targetName }}</span>
        <span class="protocol-format">{{ current.targetFormat }}</span>
      </div>
    </div>
    <div
      class="protocol-endpoints"
      aria-live="polite"
      aria-atomic="true"
    >
      <code>{{ current.endpoint }}</code><ArrowRight aria-hidden="true" /><code>{{ current.targetEndpoint }}</code>
    </div>
    <ul class="arca-feature-points protocol-points">
      <li><Activity aria-hidden="true" /><span>{{ t('site.home.protocol.streaming') }}</span></li>
      <li><Wrench aria-hidden="true" /><span>{{ t('site.home.protocol.tools') }}</span></li>
      <li><RefreshCw aria-hidden="true" /><span>{{ t('site.home.protocol.continuation') }}</span></li>
    </ul>
    <p class="arca-feature-footnote">{{ t('site.home.protocol.disclaimer') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Activity, ArrowRight, ChevronRight, Pause, Play, RefreshCw, Wrench } from 'lucide-vue-next'
import { useI18n } from './copy'
import { ARCA_LOGO_PATH, ARCA_LOGO_VIEWBOX } from './arcaBrand'
import { protocolExamples } from './feature-examples'
import { useFeatureMotion } from './useFeatureMotion'

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: false })
const { paused, running, reducedMotion, toggle } = useFeatureMotion(() => props.active)
const { t } = useI18n()
const selected = ref<string>('responses')
const current = computed(() => protocolExamples.find(protocol => protocol.id === selected.value) ?? protocolExamples[0])
</script>

<style scoped>
.protocol-flow { display: grid; grid-template-columns: minmax(210px, 260px) minmax(50px, 1fr) minmax(180px, 240px) minmax(50px, 1fr) minmax(150px, 190px); align-items: center; gap: 20px; margin-top: 56px; }
.protocol-clients { min-width: 0; padding: 0; border: 0; }
.protocol-clients legend, .protocol-column-label { font-family: var(--serif); font-size: 16px; line-height: 24px; color: var(--home-muted); }
.protocol-clients legend { margin-bottom: 16px; padding: 0; width: 100%; text-align: center; }
.protocol-option { position: relative; display: block; cursor: pointer; }
.protocol-option input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; z-index: 1; }
.protocol-option-content { display: flex; align-items: center; gap: 18px; min-height: 78px; padding: 12px 18px; border-left: 2px solid transparent; transition: background 180ms, border-color 180ms; }
.protocol-option input:checked + .protocol-option-content { border-left-color: var(--home-accent); background: var(--home-hover); }
.protocol-option input:focus-visible + .protocol-option-content { outline: 2px solid var(--home-accent); outline-offset: 3px; }
.protocol-option img { width: 32px; height: 32px; object-fit: contain; flex-shrink: 0; }
.protocol-name { display: block; font-family: var(--serif); font-size: 21px; line-height: 30px; }
.protocol-format { display: block; font-family: var(--serif); font-size: 14px; line-height: 22px; color: var(--home-muted); }
.protocol-wire { position: relative; height: 24px; overflow: hidden; margin-top: 30px; }
.protocol-wire::before { content: ''; position: absolute; top: 10px; left: 0; right: 8px; height: 3px; border-radius: 2px; background: var(--home-accent); opacity: 0.18; }
.protocol-wire svg { position: absolute; right: -1px; top: 1px; width: 22px; height: 22px; color: var(--home-accent); stroke-width: 2.5; }
.protocol-current { position: absolute; left: 0; right: 9px; top: 10px; height: 3px; border-radius: 2px; background: repeating-linear-gradient(90deg, var(--home-accent) 0 12px, transparent 12px 24px); background-size: 48px 3px; animation: protocol-current 1.2s linear infinite paused; }
.protocol-current::after { content: ''; position: absolute; top: -1px; left: -35%; width: 35%; height: 5px; border-radius: 3px; background: var(--home-accent); animation: protocol-surge 3.2s ease-in-out infinite paused; }
.wire-out .protocol-current::after { animation-delay: 1.1s; }
[data-motion='running'] .protocol-current, [data-motion='running'] .protocol-current::after, [data-motion='running'] .protocol-logo-fill, [data-motion='running'] .protocol-logo-outline, [data-motion='running'] .protocol-gateway > svg { animation-play-state: running; }
.protocol-gateway { display: flex; flex-direction: column; align-items: center; text-align: center; padding-top: 30px; }
.protocol-gateway > svg { width: 90px; height: 98px; animation: arca-drift 4s ease-in-out infinite paused; }
.protocol-logo-fill { animation: arca-fill 6s ease-in-out infinite paused; }
.protocol-logo-outline { stroke: var(--home-accent); stroke-width: 2; stroke-linecap: round; stroke-dasharray: 220 1400; animation: arca-trace 6s linear infinite paused; }
.protocol-gateway-brand { position: relative; margin-top: 8px; }
.protocol-gateway-brand .arca-motion-toggle { position: absolute; top: 50%; left: calc(100% + 8px); transform: translateY(-50%); }
.protocol-gateway-name { font-family: var(--serif); font-size: 40px; line-height: 48px; }
.protocol-gateway-caption { margin-top: 8px; font-family: var(--serif); font-size: 13px; line-height: 22px; color: var(--home-muted); }
.protocol-upstream { display: flex; flex-direction: column; align-items: center; text-align: center; min-width: 0; }
.protocol-column-label { margin-bottom: 38px; }
.protocol-upstream img { width: 72px; height: 72px; object-fit: contain; margin-bottom: 20px; }
.protocol-endpoints { display: flex; align-items: center; justify-content: center; gap: 18px; margin-top: 28px; min-height: 28px; }
.protocol-endpoints code { min-width: 0; font-family: var(--font-mono, monospace); font-size: 13px; line-height: 22px; white-space: normal !important; overflow-wrap: anywhere; }
.protocol-endpoints svg { width: 18px; height: 18px; flex-shrink: 0; color: var(--home-muted); }
.protocol-points { margin-top: 48px; }
[data-theme="dark"] .arca-protocol .is-monochrome { filter: invert(1); }
@keyframes protocol-current { to { background-position: 48px 0; } }
@keyframes protocol-surge { 0%, 10% { left: -35%; opacity: 0; } 25% { opacity: 0.8; } 90%, 100% { left: 110%; opacity: 0; } }
@keyframes arca-drift { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-4px); } }
@keyframes arca-fill { 0%, 100% { clip-path: inset(45% 0 0); } 50% { clip-path: inset(0); } }
@keyframes arca-trace { to { stroke-dashoffset: -1620; } }
@media (max-width: 1023px) {
  .protocol-flow { grid-template-columns: minmax(180px, 1.2fr) 32px minmax(150px, 1fr) 32px minmax(120px, 1fr); gap: 12px; }
  .protocol-option-content { padding-left: 12px; gap: 12px; }
  .protocol-name { font-size: 18px; }
  .protocol-gateway-caption { font-size: 11px; }
}
@media (max-width: 700px) {
  .protocol-flow { display: flex; flex-direction: column; gap: 0; margin-top: 28px; }
  .protocol-clients { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); width: 100%; }
  .protocol-clients legend { margin-bottom: 10px; font-size: 13px; }
  .protocol-option-content { flex-direction: column; justify-content: flex-start; min-height: 94px; padding: 10px 4px; gap: 6px; text-align: center; border-left: 0; border-bottom: 2px solid transparent; }
  .protocol-option input:checked + .protocol-option-content { border-bottom-color: var(--home-accent); }
  .protocol-option img { width: 24px; height: 24px; }
  .protocol-option .protocol-name { font-size: 13px; line-height: 18px; }
  .protocol-option .protocol-format { font-size: 10px; line-height: 16px; }
  .protocol-wire { width: 30px; height: 24px; margin: 6px 0; transform: rotate(90deg); }
  .protocol-gateway { padding-top: 0; }
  .protocol-gateway > svg { width: 48px; height: 52px; }
  .protocol-gateway-brand { margin-top: 2px; }
  .protocol-gateway-name { font-size: 24px; line-height: 30px; }
  .protocol-gateway-caption { margin-top: 2px; font-size: 11px; line-height: 18px; }
  .protocol-upstream { display: grid; grid-template-columns: 40px auto; column-gap: 12px; text-align: left; margin-top: 2px; }
  .protocol-column-label { grid-column: 1 / -1; margin-bottom: 8px; text-align: center; font-size: 12px; line-height: 18px; }
  .protocol-upstream img { grid-row: 2 / 4; width: 36px; height: 36px; margin: 0; }
  .protocol-upstream .protocol-name { font-size: 17px; line-height: 22px; }
  .protocol-upstream .protocol-format { font-size: 11px; line-height: 18px; }
  .protocol-endpoints { gap: 10px; margin-top: 22px; }
  .protocol-endpoints code { font-size: 10px; line-height: 18px; }
  .protocol-endpoints svg { width: 14px; height: 14px; }
  .protocol-points { margin-top: 28px; }
}
@media (prefers-reduced-motion: reduce) { *, *::before, *::after { animation: none !important; transition: none !important; } .protocol-current { display: none; } }
</style>
