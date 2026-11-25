<template>
  <div
    class="arca-feature-content arca-entitlements"
    role="group"
    :aria-label="t('site.home.entitlements.illustration')"
    :data-motion="running ? 'running' : 'paused'"
  >
    <h2
      id="entitlements-title"
      class="arca-feature-heading"
    >
      {{ t('site.home.entitlements.title') }}
    </h2>
    <p class="arca-feature-description">
      {{ t('site.home.entitlements.description') }}
    </p>
    <div class="entitlements-example">
      <div class="entitlements-example-header">
        <fieldset
          class="entitlements-profiles"
          role="radiogroup"
          :aria-label="t('site.home.entitlements.profile')"
        >
          <label
            v-for="item in exampleProfiles"
            :key="item"
          >
            <input
              v-model="profile"
              type="radio"
              name="home-profile"
              :value="item"
            >
            <span>{{ t(profileKeys[item]) }}</span>
          </label>
        </fieldset>
        <button
          class="arca-motion-toggle"
          :title="t(paused ? 'site.home.motion.play' : 'site.home.motion.pause')"
          :aria-label="t(paused ? 'site.home.motion.play' : 'site.home.motion.pause')"
          :aria-pressed="paused"
          :disabled="reducedMotion || profile === 'unlimited'"
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
      <div class="entitlements-total-header">
        <h3>{{ t('site.home.entitlements.total') }} <span>USD</span></h3>
        <fieldset
          class="entitlements-windows"
          role="radiogroup"
          :aria-label="t('site.home.entitlements.window')"
        >
          <label
            v-for="item in exampleWindows"
            :key="item"
          >
            <input
              v-model="window"
              type="radio"
              name="home-window"
              :value="item"
            >
            <span>{{ t(windowKeys[item]) }}</span>
          </label>
        </fieldset>
      </div>
      <div
        class="entitlements-amount"
        :aria-live="running ? 'off' : 'polite'"
        aria-atomic="true"
      >
        <span v-if="example.limit !== null">{{ money(example.used) }} <span class="entitlements-amount-divider">/</span> {{ money(example.limit) }}</span>
        <span
          v-else
          class="entitlements-unlimited"
        ><InfinityIcon aria-hidden="true" />{{ t('site.home.entitlements.unlimited') }}</span>
        <span
          v-if="example.limit !== null"
          class="entitlements-used-label"
        >{{ t('site.home.entitlements.used') }}</span>
      </div>
      <div
        v-if="example.limit !== null"
        class="entitlements-meter"
        role="progressbar"
        :aria-label="t('site.home.entitlements.total')"
        :aria-valuemin="0"
        :aria-valuenow="example.used"
        :aria-valuemax="example.limit"
        :aria-valuetext="`${money(example.used)} / ${money(example.limit)}`"
      >
        <span :style="{ width: `${quotaPercent(example.used, example.limit)}%` }" />
      </div>
      <div
        v-else
        class="entitlements-meter-space"
        aria-hidden="true"
      />
      <div class="entitlements-families">
        <div
          v-for="family in example.families"
          :key="family.id"
          class="entitlements-family"
          :class="`family-${family.id}`"
        >
          <span class="entitlements-family-name">{{ t('site.home.entitlements.family', { name: family.name }) }}</span>
          <div class="entitlements-family-budget">
            <span
              v-if="family.limit !== null"
              class="entitlements-family-amount"
            >{{ money(family.used) }} / {{ money(family.limit) }}</span>
            <span
              v-else
              class="entitlements-family-amount"
            >{{ t('site.home.entitlements.unlimited') }}</span>
            <div
              v-if="family.limit !== null"
              class="entitlements-meter"
              role="progressbar"
              :aria-label="t('site.home.entitlements.family', { name: family.name })"
              :aria-valuemin="0"
              :aria-valuenow="family.used"
              :aria-valuemax="family.limit"
              :aria-valuetext="`${money(family.used)} / ${money(family.limit)}`"
            >
              <span :style="{ width: `${quotaPercent(family.used, family.limit)}%` }" />
            </div>
          </div>
        </div>
      </div>
      <ul class="arca-feature-points entitlements-points">
        <li><CheckCircle2 aria-hidden="true" /><span>{{ t('site.home.entitlements.priority') }}</span></li>
        <li><CheckCircle2 aria-hidden="true" /><span>{{ t('site.home.entitlements.permissions') }}</span></li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CheckCircle2, Infinity as InfinityIcon, Pause, Play } from 'lucide-vue-next'
import { useI18n } from './copy'
import type { MessageKey } from './copy'
import { exampleProfiles, exampleWindows, getAnimatedEntitlementExample, quotaPercent, type ExampleProfile, type ExampleWindow } from './feature-examples'
import { useFeatureMotion } from './useFeatureMotion'

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: false })
const { t } = useI18n()
const profile = ref<ExampleProfile>('team')
const window = ref<ExampleWindow>('five_hour')
const elapsed = ref(0)
const { paused, running, reducedMotion, toggle } = useFeatureMotion(() => props.active && profile.value !== 'unlimited')
watch(running, (active, _, onCleanup) => {
  if (!active) return
  let frame: number
  let previous = performance.now()
  function tick(now: number) {
    elapsed.value += now - previous
    previous = now
    frame = requestAnimationFrame(tick)
  }
  frame = requestAnimationFrame(tick)
  onCleanup(() => cancelAnimationFrame(frame))
}, { immediate: true })
watch([profile, window], () => { elapsed.value = 0 })
const example = computed(() => getAnimatedEntitlementExample(profile.value, window.value, elapsed.value))
const profileKeys: Record<ExampleProfile, MessageKey> = { team: 'site.home.entitlements.team', special: 'site.home.entitlements.special', unlimited: 'site.home.entitlements.unlimitedMember' }
const windowKeys: Record<ExampleWindow, MessageKey> = { five_hour: 'site.home.entitlements.fiveHour', weekly: 'site.home.entitlements.weekly', monthly: 'site.home.entitlements.monthly' }
const currency = new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'USD', currencyDisplay: 'narrowSymbol', maximumFractionDigits: 0 })
function money(value: number) { return currency.format(value) }
</script>

<style scoped>
.entitlements-example { width: min(100%, 760px); margin: 44px auto 0; }
.entitlements-example-header { display: flex; justify-content: space-between; align-items: center; gap: 16px; min-height: 44px; padding-bottom: 12px; border-bottom: 1px solid var(--home-border); font-family: var(--sans-serif); font-size: 13px; line-height: 20px; color: var(--home-muted); }
.entitlements-profiles, .entitlements-windows { display: flex; gap: 18px; border: 0; min-width: 0; padding: 0; }
.entitlements-profiles label, .entitlements-windows label { position: relative; cursor: pointer; }
input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; z-index: 1; }
.entitlements-profiles span { display: block; padding: 8px 0; border-bottom: 2px solid transparent; white-space: nowrap; }
.entitlements-profiles input:checked + span { color: var(--home-accent); border-bottom-color: var(--home-accent); }
input:focus-visible + span { outline: 2px solid var(--home-accent); outline-offset: 3px; }
.entitlements-total-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 24px; }
.entitlements-total-header h3 { font-family: var(--serif); font-size: 18px; line-height: 28px; font-weight: 500; }
.entitlements-total-header h3 span { margin-left: 7px; color: var(--home-muted); font-family: var(--sans-serif); font-size: 10px; }
.entitlements-windows { gap: 0; flex-shrink: 0; border: 1px solid var(--home-border); border-radius: 6px; overflow: hidden; font-family: var(--sans-serif); font-size: 12px; line-height: 18px; }
.entitlements-windows span { display: block; min-width: 60px; padding: 7px 12px; text-align: center; color: var(--home-muted); white-space: nowrap; }
.entitlements-windows label + label { border-left: 1px solid var(--home-border); }
.entitlements-windows input:checked + span { color: var(--home-background); background: var(--home-accent); }
.entitlements-windows input:focus-visible + span { outline-offset: -3px; }
.entitlements-amount { display: flex; align-items: baseline; gap: 18px; min-height: 74px; padding-top: 16px; font-family: var(--serif); font-size: 42px; line-height: 58px; }
.entitlements-amount-divider { padding: 0 5px; }
.entitlements-amount .entitlements-used-label { font-size: 14px; line-height: 22px; color: var(--home-muted); }
.entitlements-unlimited { display: flex; align-items: center; gap: 12px; font-size: 32px; }
.entitlements-unlimited svg { width: 38px; height: 38px; stroke-width: 1.5; }
.entitlements-meter { height: 7px; overflow: hidden; border-radius: 4px; background: var(--home-border); }
.entitlements-meter-space { height: 7px; }
.entitlements-meter > span { display: block; height: 100%; border-radius: inherit; background: var(--pool-accent, var(--home-accent)); transition: width 300ms ease; }
[data-motion='running'] .entitlements-meter > span { transition: none; }
.entitlements-families { position: relative; display: flex; flex-direction: column; gap: 24px; margin: 30px 14px 0 32px; padding-left: 24px; border-left: 1px solid var(--home-border); }
.entitlements-family { position: relative; display: grid; grid-template-columns: 140px minmax(0, 1fr); align-items: start; gap: 16px; }
.entitlements-family::before { content: ''; position: absolute; left: -24px; top: 13px; width: 16px; height: 1px; background: var(--home-border); }
.entitlements-family-name { font-family: var(--serif); font-size: 20px; line-height: 28px; }
.entitlements-family-budget { min-width: 0; min-height: 41px; }
.entitlements-family-amount { font-family: var(--serif); font-size: 18px; line-height: 28px; color: var(--pool-accent, var(--home-accent)); }
.entitlements-family .entitlements-meter { margin-top: 8px; height: 5px; }
.family-astra { --pool-accent: #728f89; }
.entitlements-points { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 28px; }
@media (max-width: 700px) {
  .entitlements-example { margin-top: 28px; }
  .entitlements-example-header { align-items: center; gap: 8px; padding-bottom: 6px; font-size: 12px; }
  .entitlements-profiles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; flex: 1; text-align: center; }
  .entitlements-profiles span { display: flex; justify-content: center; align-items: center; min-height: 48px; padding: 6px 0; white-space: normal; font-size: 12px; line-height: 18px; }
  .entitlements-total-header { margin-top: 18px; gap: 8px; }
  .entitlements-total-header h3 { font-size: 16px; }
  .entitlements-total-header h3 span { margin-left: 4px; font-size: 9px; }
  .entitlements-windows { font-size: 11px; }
  .entitlements-windows span { min-width: 48px; padding: 6px 8px; }
  .entitlements-amount { font-size: 32px; line-height: 44px; min-height: 64px; gap: 12px; flex-wrap: wrap; }
  .entitlements-amount .entitlements-used-label { font-size: 12px; }
  .entitlements-families { margin: 24px 0 0 10px; padding-left: 16px; gap: 20px; }
  .entitlements-family { grid-template-columns: 94px minmax(0, 1fr); gap: 10px; }
  .entitlements-family::before { left: -16px; width: 10px; top: 11px; }
  .entitlements-family-name { font-size: 16px; line-height: 24px; }
  .entitlements-family-amount { font-size: 15px; line-height: 24px; }
  .entitlements-family-budget { min-height: 37px; }
  .entitlements-points { margin-top: 22px; }
}
@media (max-width: 359px) {
  .entitlements-total-header { flex-direction: column; align-items: stretch; }
  .entitlements-windows { width: 100%; }
  .entitlements-windows label { flex: 1; min-width: 0; }
}
@media (prefers-reduced-motion: reduce) { * { transition: none !important; } }
</style>
