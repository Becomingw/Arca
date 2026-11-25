<template>
  <div
    ref="scrollContainer"
    class="arca-home"
    @scroll.passive="handleScroll"
  >
    <header
      ref="header"
      class="home-header"
    >
      <button
        class="home-brand"
        :aria-label="t('site.home.section.home')"
        @click="scrollToSection(0)"
      >
        <svg class="home-brand-mark" :viewBox="ARCA_LOGO_VIEWBOX" aria-hidden="true">
          <path :d="ARCA_LOGO_PATH" fill="currentColor" />
        </svg>
        <span class="home-brand-copy">
          <span class="home-brand-name">{{ ARCA_NAME }}</span>
          <span class="home-brand-subtitle">{{ ARCA_SUBTITLE }}</span>
        </span>
      </button>
      <nav
        class="home-header-nav"
        :aria-label="t('site.home.harness.navigation')"
      >
        <button
          v-for="index in [0, 1, 2, 3, 4]"
          :key="index"
          :class="{ selected: currentSection === index }"
          @click="scrollToSection(index)"
        >
          {{ pageLabel(index) }}
        </button>
        <a :href="ARCA_DOCS_URL" target="_blank" rel="noopener noreferrer">
          {{ t('site.home.docLink') }}
        </a>
      </nav>
      <div class="home-header-actions">
        <a :href="authenticated ? '/arca/' : signInPath" class="home-entry">
          {{ t(authenticated ? 'site.home.enterDashboard' : 'site.home.login') }}
        </a>
        <button
          class="home-theme"
          type="button"
          :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
          @click="$emit('toggle-theme')"
        >
          <Sun v-if="theme === 'dark'" aria-hidden="true" />
          <Moon v-else aria-hidden="true" />
        </button>
        <a
          :href="ARCA_REPOSITORY_URL"
          target="_blank"
          rel="noopener noreferrer"
          class="home-github"
          :title="t('common.githubRepository')"
          :aria-label="t('common.githubRepository')"
        >
          <Github aria-hidden="true" />
        </a>
      </div>
    </header>
    <nav
      class="home-pagination"
      :aria-label="t('site.home.harness.pages')"
    >
      <button
        v-for="index in PAGE_COUNT"
        :key="index"
        :aria-label="pageLabel(index - 1)"
        :title="pageLabel(index - 1)"
        :aria-current="currentSection === index - 1 ? 'page' : undefined"
        @click="scrollToSection(index - 1)"
      >
        <span :class="{ selected: currentSection === index - 1 }" />
      </button>
    </nav>
    <main>
      <section
        id="home-section-0"
        :ref="el => setSectionRef(el, 0)"
        class="home-page home-hero"
        aria-labelledby="home-title"
      >
        <div class="home-hero-content">
          <svg
            class="home-hero-mark"
            :viewBox="ARCA_LOGO_VIEWBOX"
            aria-hidden="true"
          ><path
            :d="ARCA_LOGO_PATH"
            fill="currentColor"
          /></svg>
          <h1 id="home-title">
            {{ ARCA_NAME }}
          </h1>
          <p class="home-hero-description">
            {{ t('site.home.harness.tagline') }}
          </p>
          <HarnessCarousel :active="currentSection === 0" />
          <p v-if="message" role="alert" class="error home-error">{{ message }}</p>
        </div>
        <button
          class="home-next"
          :title="pageLabel(1)"
          :aria-label="pageLabel(1)"
          @click="scrollToSection(1)"
        >
          <ChevronDown aria-hidden="true" />
        </button>
      </section>
      <section
        id="home-section-1"
        :ref="el => setSectionRef(el, 1)"
        class="home-page arca-feature-page"
        aria-labelledby="protocol-title"
      >
        <ProtocolFeature :active="currentSection === 1" />
        <button
          class="home-next"
          :title="pageLabel(2)"
          :aria-label="pageLabel(2)"
          @click="scrollToSection(2)"
        >
          <ChevronDown aria-hidden="true" />
        </button>
      </section>
      <section
        id="home-section-2"
        :ref="el => setSectionRef(el, 2)"
        class="home-page arca-feature-page"
        aria-labelledby="entitlements-title"
      >
        <EntitlementsFeature :active="currentSection === 2" />
        <button
          class="home-next"
          :title="pageLabel(3)"
          :aria-label="pageLabel(3)"
          @click="scrollToSection(3)"
        >
          <ChevronDown aria-hidden="true" />
        </button>
      </section>
      <section
        id="home-section-3"
        :ref="el => setSectionRef(el, 3)"
        class="home-page arca-feature-page"
        aria-labelledby="operations-title"
      >
        <OperationsFeature
          :active="currentSection === 3"
          :feedback-path="authenticated ? '/arca/#feedback' : '/sign-in?redirect=%2Farca%2F%23feedback'"
        />
        <button
          class="home-next"
          :title="pageLabel(4)"
          :aria-label="pageLabel(4)"
          @click="scrollToSection(4)"
        >
          <ChevronDown aria-hidden="true" />
        </button>
      </section>
      <section
        id="home-section-4"
        :ref="el => setSectionRef(el, 4)"
        class="home-page arca-feature-page"
        aria-labelledby="friend-links-title"
      >
        <FriendLinksFeature @home="scrollToSection(0)" />
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, type ComponentPublicInstance } from 'vue'
import { ChevronDown, Github, Moon, Sun } from 'lucide-vue-next'
import { useI18n } from './copy'
import { ARCA_DOCS_URL, ARCA_LOGO_PATH, ARCA_LOGO_VIEWBOX, ARCA_NAME, ARCA_REPOSITORY_URL, ARCA_SUBTITLE } from './arcaBrand'
import HarnessCarousel from './HarnessCarousel.vue'
import ProtocolFeature from './ProtocolFeature.vue'
import EntitlementsFeature from './EntitlementsFeature.vue'
import OperationsFeature from './OperationsFeature.vue'
import FriendLinksFeature from './FriendLinksFeature.vue'
import './home-features.css'

defineProps<{ authenticated: boolean, theme: string, message: string }>()
defineEmits<{ 'toggle-theme': [] }>()

const PAGE_COUNT = 5
const signInPath = '/sign-in?redirect=%2Farca%2F'
const { t } = useI18n()
const scrollContainer = ref<HTMLElement | null>(null)
const header = ref<HTMLElement | null>(null)
const currentSection = ref(0)
const sectionElements: Array<HTMLElement | null> = []

function pageLabel(index: number) {
  if (index === 0) return t('site.home.section.home')
  if (index === 1) return t('site.home.protocol.nav')
  if (index === 2) return t('site.home.entitlements.nav')
  if (index === 3) return t('site.home.operations.nav')
  if (index === 4) return t('site.home.links.nav')
  return t('site.home.harness.page', { page: index + 1 })
}
function setSectionRef(element: Element | ComponentPublicInstance | null, index: number) {
  sectionElements[index] = element instanceof HTMLElement ? element : null
}
function handleScroll() {
  const container = scrollContainer.value
  if (!container) return
  const middle = container.scrollTop + container.clientHeight / 2
  for (let index = PAGE_COUNT - 1; index >= 0; index--) {
    const section = sectionElements[index]
    if (section && section.offsetTop <= middle) {
      currentSection.value = index
      break
    }
  }
}
function scrollToSection(index: number) {
  const target = sectionElements[index]
  if (!target) return
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  scrollContainer.value?.scrollTo({ top: target.offsetTop - (header.value?.offsetHeight ?? 0), behavior: reducedMotion ? 'instant' : 'smooth' })
}
</script>

<style scoped>
.arca-home {
  --serif: Georgia, "Songti SC", "Noto Serif CJK SC", serif;
  --sans-serif: Inter, "PingFang SC", "Noto Sans SC", system-ui, sans-serif;
  --home-grid: rgba(61, 57, 41, 0.035);
  --home-background: #fafaf7;
  --home-text: #191714;
  --home-muted: #898882;
  --home-accent: #cc785c;
  --home-border: rgba(61, 57, 41, 0.1);
  --home-hover: rgba(204, 120, 92, 0.08);
  --home-header-height: 72px;
  position: relative;
  height: 100dvh;
  max-height: 100%;
  container-type: size;
  overflow-y: auto;
  overflow-x: hidden;
  scroll-snap-type: y mandatory;
  scroll-padding-top: var(--home-header-height);
  color: var(--home-text);
  background-color: var(--home-background);
  background-image: linear-gradient(var(--home-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--home-grid) 1px, transparent 1px);
  background-size: 24px 24px;
  letter-spacing: 0;
}
[data-theme="dark"] .arca-home {
  --home-grid: rgba(250, 249, 245, 0.045);
  --home-background: #191714;
  --home-text: #fafaf7;
  --home-muted: #a5a198;
  --home-accent: #d4a27f;
  --home-border: rgba(227, 224, 211, 0.13);
  --home-hover: rgba(212, 162, 127, 0.1);
}
.arca-home::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
  opacity: .025;
  pointer-events: none;
}
.home-brand-mark { width: 36px; height: 36px; }
.home-error { margin-top: 14px; }
.home-header { position: sticky; top: 0; z-index: 50; display: flex; align-items: center; justify-content: space-between; gap: 24px; min-height: var(--home-header-height); padding: 0 6%; border-bottom: 1px solid var(--home-border); background: var(--home-background); }
.home-brand { display: inline-flex; align-items: center; gap: 12px; text-align: left; flex-shrink: 0; }
.home-brand-copy { display: flex; flex-direction: column; gap: 5px; }
.home-brand-name { font-family: var(--serif); font-size: 23px; line-height: 26px; }
.home-brand-subtitle { font-family: var(--sans-serif); font-size: 11px; line-height: 14px; color: var(--home-muted); }
.home-header-nav { display: flex; align-items: center; gap: 28px; position: absolute; left: 50%; transform: translateX(-50%); }
.home-header-nav a, .home-header-nav button { padding: 10px 0; font-family: var(--sans-serif); font-size: 14px; line-height: 20px; border-bottom: 2px solid transparent; }
.home-header-nav .selected { color: var(--home-accent); border-bottom-color: var(--home-accent); }
.home-header-actions { display: flex; align-items: center; gap: 12px; }
.home-entry { display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 8px 16px; border-radius: 6px; background: var(--home-text); color: var(--home-background); font-family: var(--sans-serif); font-size: 13px; font-weight: 500; line-height: 20px; white-space: nowrap; transition: opacity 160ms; }
.home-entry:hover { opacity: 0.8; }
.home-github, .home-theme { display: grid; place-items: center; width: 32px; height: 32px; color: var(--home-muted); }
.home-github svg, .home-theme svg { width: 18px; height: 18px; }
button:focus-visible, a:focus-visible { outline: 2px solid var(--home-accent); outline-offset: 4px; border-radius: 6px; }
.home-page { position: relative; min-height: calc(100cqh - var(--home-header-height)); scroll-snap-align: start; }
.home-hero { display: flex; flex-direction: column; justify-content: center; align-items: center; padding: 48px 24px 100px; }
.home-hero-content { display: flex; flex-direction: column; align-items: center; width: min(100%, 880px); }
.home-hero-mark { width: 154px; height: 166px; flex-shrink: 0; animation: mark-arrive 700ms ease-out both; }
.home-hero h1 { margin: 24px 0 0; font-family: var(--serif); font-size: 80px; font-weight: 500; line-height: 1.15; letter-spacing: 0; text-align: center; overflow-wrap: anywhere; max-width: 100%; }
.home-hero-description { margin: 10px 0 0; font-family: var(--serif); font-size: 22px; line-height: 34px; color: var(--home-muted); text-align: center; }
.home-hero :deep(.harness-carousel) { margin-top: 52px; }
.home-next { position: absolute; bottom: 24px; left: 50%; transform: translateX(-50%); display: grid; place-items: center; width: 40px; height: 40px; color: var(--home-muted); }
.home-next svg { width: 22px; height: 22px; stroke-width: 1.75; }
.home-pagination { position: fixed; right: 30px; top: 50%; transform: translateY(-50%); z-index: 40; display: flex; flex-direction: column; gap: 4px; }
.home-pagination button { display: grid; place-items: center; width: 28px; height: 28px; }
.home-pagination span { width: 9px; height: 9px; border-radius: 50%; background: var(--home-muted); opacity: 0.28; transition: background 200ms, opacity 200ms; }
.home-pagination .selected { opacity: 1; background: var(--home-accent); }
@keyframes mark-arrive { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
@media (max-width: 1023px) {
  .home-header { padding: 0 24px; }
  .home-header-nav { position: static; transform: none; }
  .home-pagination { right: 6px; }
  .home-hero h1 { font-size: 68px; }
}
@media (max-width: 1100px) { .home-header-nav { display: none; } }
@media (max-width: 767px) {
  .arca-home { --home-header-height: 60px; }
  .home-header { gap: 8px; padding: 0 14px; }
  .home-brand { gap: 8px; }
  .home-brand :deep(svg) { width: 28px; height: 28px; }
  .home-brand :deep(.relative) { width: 28px; height: 28px; }
  .home-brand-name { font-size: 18px; line-height: 20px; }
  .home-brand-subtitle { font-size: 9px; line-height: 12px; }
  .home-brand-copy { gap: 2px; }
  .home-header-nav { display: none; }
  .home-header-actions { gap: 3px; }
  .home-entry { min-height: 32px; padding: 5px 10px; font-size: 11px; }
  .home-github { width: 28px; height: 28px; }
  .home-github svg { width: 15px; height: 15px; }
  .home-pagination { right: 0; }
  .home-pagination button { width: 22px; height: 26px; }
  .home-pagination span { width: 5px; height: 5px; }
  .home-hero { padding: 32px 22px 76px; }
  .home-hero-mark { width: 104px; height: 112px; }
  .home-hero h1 { font-size: 56px; margin-top: 18px; }
  .home-hero-description { font-size: 17px; line-height: 28px; margin-top: 8px; }
  .home-hero :deep(.harness-carousel) { margin-top: 34px; }
  .home-next { bottom: 16px; }
}
@media (max-width: 359px) {
  .home-header { padding: 0 9px; }
  .home-brand-subtitle { display: none; }
  .home-github { display: none; }
  .home-hero { padding-left: 16px; padding-right: 16px; }
}
@media (max-height: 760px) {
  .home-hero { padding-top: 24px; padding-bottom: 60px; }
  .home-hero-mark { width: 92px; height: 100px; }
  .home-hero h1 { font-size: 56px; margin-top: 14px; }
  .home-hero-description { font-size: 18px; line-height: 28px; }
  .home-hero :deep(.harness-carousel) { margin-top: 24px; }
  .home-next { bottom: 10px; }
}
@media (max-height: 600px) {
  .home-hero-mark { width: 62px; height: 68px; }
  .home-hero h1 { font-size: 42px; margin-top: 8px; }
  .home-hero-description { font-size: 16px; line-height: 24px; }
  .home-hero :deep(.harness-carousel) { margin-top: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before { animation: none !important; transition: none !important; }
}
</style>
