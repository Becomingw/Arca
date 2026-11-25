export const protocolExamples = [
  { id: 'responses', name: 'OpenAI', format: 'Responses', icon: import.meta.env.BASE_URL + 'openai.svg', monochrome: true, endpoint: '/v1/responses', targetName: 'Claude', targetFormat: 'Messages', targetIcon: import.meta.env.BASE_URL + 'claude-color.svg', targetMonochrome: false, targetEndpoint: '/v1/messages' },
  { id: 'messages', name: 'Claude', format: 'Messages', icon: import.meta.env.BASE_URL + 'claude-color.svg', monochrome: false, endpoint: '/v1/messages', targetName: 'OpenAI', targetFormat: 'Chat Completions', targetIcon: import.meta.env.BASE_URL + 'openai.svg', targetMonochrome: true, targetEndpoint: '/v1/chat/completions' },
  { id: 'chat', name: 'OpenAI', format: 'Chat Completions', icon: import.meta.env.BASE_URL + 'openai.svg', monochrome: true, endpoint: '/v1/chat/completions', targetName: 'OpenAI', targetFormat: 'Responses', targetIcon: import.meta.env.BASE_URL + 'openai.svg', targetMonochrome: true, targetEndpoint: '/v1/responses' },
  { id: 'gemini', name: 'Gemini', format: 'Generate Content', icon: import.meta.env.BASE_URL + 'gemini-color.svg', monochrome: false, endpoint: '/v1beta/models/{model}:generateContent', targetName: 'OpenAI', targetFormat: 'Chat Completions', targetIcon: import.meta.env.BASE_URL + 'openai.svg', targetMonochrome: true, targetEndpoint: '/v1/chat/completions' },
] as const

export type ExampleProfile = 'team' | 'special' | 'unlimited'
export type ExampleWindow = 'five_hour' | 'weekly' | 'monthly'
export const exampleProfiles: ExampleProfile[] = ['team', 'special', 'unlimited']
export const exampleWindows: ExampleWindow[] = ['five_hour', 'weekly', 'monthly']

const budgets = {
  five_hour: { used: 6, specialUsed: 8, group: 20, special: 40, fabelUsed: 3, fabel: 5, astraUsed: 2, astra: 8 },
  weekly: { used: 72, specialUsed: 96, group: 200, special: 400, fabelUsed: 30, fabel: 60, astraUsed: 22, astra: 90 },
  monthly: { used: 210, specialUsed: 360, group: 600, special: 1200, fabelUsed: 90, fabel: 150, astraUsed: 70, astra: 240 },
} as const

// Public illustrations only: these values are never loaded from an account or written to a policy.
export function getEntitlementExample(profile: ExampleProfile, window: ExampleWindow) {
  const budget = budgets[window]
  const unlimited = profile === 'unlimited'
  const used = profile === 'special' ? budget.specialUsed : budget.used
  const limit = unlimited ? null : profile === 'special' ? budget.special : budget.group
  return {
    used,
    limit,
    groupLimit: budget.group,
    otherUsed: used - budget.fabelUsed - budget.astraUsed,
    families: [
      { id: 'fabel', name: 'Fabel', used: budget.fabelUsed, limit: unlimited ? null : budget.fabel },
      { id: 'astra', name: 'Astra', used: budget.astraUsed, limit: unlimited ? null : budget.astra },
    ],
  }
}

export const USAGE_ANIMATION_MS = 12000
export const USAGE_RISE_MS = 1000

export function getAnimatedEntitlementExample(profile: ExampleProfile, window: ExampleWindow, elapsed: number) {
  const base = getEntitlementExample(profile, window)
  const cycleTime = Math.max(0, elapsed) % USAGE_ANIMATION_MS
  const rising = cycleTime <= USAGE_RISE_MS
  const phase = rising
    ? cycleTime / USAGE_RISE_MS
    : (cycleTime - USAGE_RISE_MS) / (USAGE_ANIMATION_MS - USAGE_RISE_MS)
  const progress = (1 - Math.cos(phase * Math.PI)) / 2
  const multiplier = profile === 'unlimited' ? 1 : 1 + 0.6 * (rising ? progress : 1 - progress)
  const scale = (value: number) => Math.round(value * multiplier * 100) / 100
  const families = base.families.map(family => ({ ...family, used: scale(family.used) }))
  const otherUsed = scale(base.otherUsed)
  return { ...base, families, otherUsed, used: Math.round((families.reduce((sum, family) => sum + family.used, 0) + otherUsed) * 100) / 100 }
}

export function quotaPercent(used: number, limit: number): number {
  return limit > 0 ? Math.min(100, Math.max(0, used / limit * 100)) : 0
}
