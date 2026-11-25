<template>
  <div class="arca-feature-content arca-friend-links">
    <h2
      id="friend-links-title"
      class="arca-feature-heading"
    >
      {{ t('site.home.links.title') }}
    </h2>
    <p class="arca-feature-description">
      {{ t('site.home.links.description') }}
    </p>
    <a
      class="friend-institution"
      :href="institutionLink.href"
      target="_blank"
      rel="noopener noreferrer"
      :aria-label="t('site.home.links.open', { name: institutionLink.name })"
      :title="t('site.home.links.open', { name: institutionLink.name })"
    >
      <Ai4RadMark
        class="friend-institution-icon"
        aria-hidden="true"
      />
      <span class="friend-institution-copy">
        <span class="friend-institution-name">{{ institutionLink.name }}</span>
        <span class="friend-institution-description">{{ t(institutionLink.description) }}</span>
      </span>
      <span class="friend-institution-label">{{ t('site.home.links.institution') }}</span>
      <ArrowUpRight
        class="friend-outbound"
        aria-hidden="true"
      />
    </a>
    <div class="friend-groups">
      <section
        v-for="group in friendLinkGroups"
        :key="group.id"
        class="friend-group"
        :aria-labelledby="`friend-group-${group.id}`"
      >
        <h3 :id="`friend-group-${group.id}`">
          {{ t(group.title) }}
        </h3>
        <ul>
          <li
            v-for="link in group.links"
            :key="link.href"
          >
            <a
              class="friend-project"
              :class="{ 'friend-acknowledgement': 'acknowledgement' in link && link.acknowledgement }"
              :href="link.href"
              target="_blank"
              rel="noopener noreferrer"
              :aria-label="t('site.home.links.open', { name: link.name })"
              :title="t('site.home.links.open', { name: link.name })"
            >
              <HeartHandshake
                v-if="'acknowledgement' in link && link.acknowledgement"
                class="friend-credit-icon"
                aria-hidden="true"
              />
              <span class="friend-project-copy">
                <span class="friend-project-name">{{ link.name }}</span>
                <span class="friend-project-description">{{ t(link.description) }}</span>
              </span>
              <ArrowUpRight
                class="friend-outbound"
                aria-hidden="true"
              />
            </a>
          </li>
        </ul>
      </section>
    </div>
    <footer class="friend-footer">
      <span>{{ ARCA_NAME }} · {{ t('site.home.links.hub') }}</span>
      <button
        class="friend-return"
        :title="t('site.home.links.return')"
        :aria-label="t('site.home.links.return')"
        @click="$emit('home')"
      >
        <ArrowUp aria-hidden="true" />
      </button>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ArrowUp, ArrowUpRight, HeartHandshake } from 'lucide-vue-next'
import { useI18n } from './copy'
import { ARCA_NAME } from './arcaBrand'
import Ai4RadMark from './Ai4RadMark.vue'
import { friendLinkGroups, institutionLink } from './friend-links'

defineEmits<{ home: [] }>()
const { t } = useI18n()
</script>

<style scoped>
.friend-institution { display: grid; grid-template-columns: 60px minmax(0, 1fr) auto 22px; align-items: center; gap: 24px; min-height: 108px; margin-top: 36px; padding: 20px 12px; border-block: 1px solid var(--home-border); }
.friend-institution-icon { --ai4rad-ink: var(--home-text); --ai4rad-teal: #617c76; --ai4rad-accent: var(--home-accent); width: 60px; height: 60px; }
.friend-institution-copy, .friend-project-copy { display: flex; flex-direction: column; min-width: 0; gap: 5px; }
.friend-institution-name { font-family: var(--serif); font-size: 28px; line-height: 36px; }
.friend-institution-description { font-family: var(--serif); font-size: 15px; line-height: 24px; color: var(--home-muted); }
.friend-institution-label { font-family: var(--serif); font-size: 14px; line-height: 22px; white-space: nowrap; }
.friend-outbound { width: 20px; height: 20px; stroke-width: 1.5; transition: color 180ms, transform 180ms; }
.friend-groups { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr); gap: 64px; margin-top: 32px; }
.friend-group h3 { padding-bottom: 14px; border-bottom: 1px solid var(--home-border); font-family: var(--serif); font-size: 16px; line-height: 24px; font-weight: 500; color: #617c76; }
.friend-group ul { padding: 0; margin: 0; list-style: none; }
.friend-project { display: grid; grid-template-columns: minmax(0, 1fr) 20px; align-items: center; gap: 20px; min-height: 82px; padding: 15px 8px 15px 0; border-bottom: 1px solid var(--home-border); }
.friend-project-name { font-family: var(--serif); font-size: 22px; line-height: 29px; overflow-wrap: anywhere; }
.friend-project-description { font-family: var(--serif); font-size: 14px; line-height: 22px; color: var(--home-muted); overflow-wrap: anywhere; }
.friend-group li:last-child .friend-project { border-bottom: 0; }
.friend-acknowledgement { grid-template-columns: 30px minmax(0, 1fr) 20px; margin-top: 12px; }
.friend-credit-icon { width: 30px; height: 30px; stroke-width: 1.3; color: var(--home-accent); }
.friend-institution:hover, .friend-project:hover { color: var(--home-accent); }
.friend-institution:hover .friend-outbound, .friend-project:hover .friend-outbound { transform: translate(2px, -2px); }
.friend-institution:focus-visible, .friend-project:focus-visible, .friend-return:focus-visible { outline: 2px solid var(--home-accent); outline-offset: 4px; border-radius: 4px; }
.friend-footer { display: flex; align-items: center; justify-content: space-between; gap: 20px; margin-top: 28px; padding-top: 20px; border-top: 1px solid var(--home-border); font-family: var(--serif); font-size: 16px; line-height: 24px; }
.friend-return { display: grid; place-items: center; width: 38px; height: 38px; border: 1px solid var(--home-border); border-radius: 50%; transition: color 180ms, border-color 180ms; }
.friend-return svg { width: 20px; height: 20px; stroke-width: 1.5; }
.friend-return:hover { color: var(--home-accent); border-color: var(--home-accent); }
[data-theme="dark"] .friend-group h3 { color: #9bb6ae; }
[data-theme="dark"] .friend-institution-icon { --ai4rad-teal: #9bb6ae; }
@media (max-width: 1023px) { .friend-groups { gap: 36px; } }
@media (max-width: 700px) {
  .friend-institution { grid-template-columns: 44px minmax(0, 1fr) 20px; gap: 14px; margin-top: 28px; padding: 20px 0; }
  .friend-institution-icon { width: 44px; height: 44px; }
  .friend-institution-name { font-size: 24px; line-height: 30px; }
  .friend-institution-description { font-size: 13px; line-height: 22px; }
  .friend-institution-label { display: none; }
  .friend-groups { grid-template-columns: minmax(0, 1fr); gap: 28px; margin-top: 28px; }
  .friend-group h3 { font-size: 15px; padding-bottom: 12px; }
  .friend-project { min-height: 86px; gap: 16px; padding: 16px 4px 16px 0; }
  .friend-project-name { font-size: 21px; line-height: 28px; }
  .friend-project-description { font-size: 13px; line-height: 22px; }
  .friend-acknowledgement { margin-top: 8px; }
  .friend-footer { margin-top: 26px; font-size: 15px; }
}
@media (prefers-reduced-motion: reduce) { * { transition: none !important; } }
</style>
