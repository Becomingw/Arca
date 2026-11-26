/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { readCachedStatus } from '@/lib/status-query'

export type CCSwitchUsageLabels = {
  locale?: string
  unit: string
  subscription: string
  periods: Record<string, string>
  scopes: Record<string, string>
  states: Record<string, string>
  resetAt: string
  reserved: string
  temporary: string
  allModels: string
  startsAt: string
  expiresAt: string
  neverExpires: string
  queryFailed: string
}

function parseGatewayAddress(value: unknown): URL | null {
  if (typeof value !== 'string' || !value) return null
  try {
    const url = new URL(value)
    if (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      !url.username &&
      !url.password &&
      !url.pathname.startsWith('//') &&
      !url.search &&
      !url.hash
    ) {
      return url
    }
  } catch {
    return null
  }
  return null
}

function isLoopback(hostname: string): boolean {
  return (
    hostname === 'localhost' ||
    hostname === '[::1]' ||
    /^127(?:\.\d{1,3}){3}$/.test(hostname)
  )
}

export function resolveCCSwitchGateway(openCodeEndpoint?: string): URL {
  const page = new URL(window.location.origin)
  const configured = parseGatewayAddress(
    openCodeEndpoint ?? readCachedStatus()?.server_address
  )
  if (!configured) return page
  // The fresh server advertises its private container address. Preserve any
  // real deployment origin and path, including an HTTPS override.
  if (
    configured.protocol === 'http:' &&
    isLoopback(configured.hostname) &&
    configured.port === '3000'
  ) {
    page.pathname = configured.pathname
    return page
  }
  return configured
}

export function supportsCCSwitchUsage(gateway: URL): boolean {
  return (
    gateway.protocol === 'https:' ||
    (gateway.protocol === 'http:' && isLoopback(gateway.hostname))
  )
}

function encodeUsageScript(script: string): string {
  const bytes = new TextEncoder().encode(script)
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
  return btoa(binary)
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/=+$/, '')
}

type CCSwitchImport = {
  app: string
  name: string
  models: Record<string, string>
  apiKey: string
  gateway: URL
  usage?: CCSwitchUsageLabels
}

function buildUsageScript(labels: CCSwitchUsageLabels): string {
  return `({
  request: {
    url: "{{baseUrl}}/api/ccswitch/usage",
    method: "GET",
    headers: { "Authorization": "Bearer {{apiKey}}", "Accept": "application/json" }
  },
  extractor: function (response) {
    const labels = ${JSON.stringify(labels)};
    if (!response || response.success !== true || !response.data) {
      return { isValid: false, invalidMessage: labels.queryFailed };
    }
    const data = response.data;
    const plan = data.subscription_name || labels.subscription;
    const rows = [];
    const units = (micros) => micros / 1000000;
    const timestamp = (seconds) => {
      const date = new Date(seconds * 1000);
      if (typeof Intl !== "undefined" && typeof Intl.DateTimeFormat === "function") {
        try {
          return new Intl.DateTimeFormat(labels.locale, {
            dateStyle: "medium", timeStyle: "short", timeZone: data.timezone || "UTC"
          }).format(date) + (data.timezone ? ' (' + data.timezone + ')' : ' UTC');
        } catch (_) {}
      }
      return date.toISOString().slice(0, 16).replace("T", " ") + " UTC";
    };
    if (labels.states[data.state]) {
      rows.push({ planName: plan, isValid: true, extra: labels.states[data.state] });
    }
    for (const window of (data.windows || [])) {
      const scope = labels.scopes[window.scope];
      const period = labels.periods[window.period] || window.period;
      const note = [];
      if (window.end > 0 && window.period !== "rolling_5h") {
        note.push(labels.resetAt + ' ' + timestamp(window.end));
      }
      if (window.reserved_micros > 0) {
        note.push(labels.reserved + ': ' + units(window.reserved_micros) + ' ' + labels.unit);
      }
      rows.push({
        planName: [plan, scope && window.name ? scope + ': ' + window.name : '', period].filter(Boolean).join(' · '),
        isValid: true,
        remaining: units(window.remaining_micros),
        used: units(window.used_micros),
        total: units(window.limit_micros),
        unit: labels.unit,
        extra: note.join(' · ')
      });
    }
    if (rows.length === 0) {
      rows.push({ planName: plan, isValid: true, extra: labels.states.noWindows });
    }
    for (const grant of (data.grants || [])) {
      const note = [];
      if (grant.family) note.push(labels.scopes.family + ': ' + grant.family);
      if (grant.models && grant.models.length) note.push(grant.models.join(', '));
      else if (!grant.family) note.push(labels.allModels);
      if (grant.starts_at > Date.now() / 1000) {
        note.push(labels.startsAt.replace("%s", timestamp(grant.starts_at)));
      }
      note.push(grant.ends_at > 0 ? labels.expiresAt + ' ' + timestamp(grant.ends_at) : labels.neverExpires);
      rows.push({
        planName: labels.temporary + ' · ' + grant.name,
        isValid: true,
        remaining: units(grant.remaining_micros),
        total: units(grant.amount_micros),
        unit: labels.unit,
        extra: note.join(' · ')
      });
    }
    return rows;
  }
})`
}

export function buildCCSwitchURL(input: CCSwitchImport): string {
  const address = input.gateway.href.replace(/\/+$/, '')
  const base = address.replace(/\/v1$/, '')
  let endpoint = base
  if (input.app === 'codex') endpoint = `${base}/v1`
  if (input.app === 'opencode') endpoint = address

  const params = new URLSearchParams({
    resource: 'provider',
    app: input.app,
    name: input.name,
    endpoint,
    apiKey: input.apiKey,
    homepage: base,
    enabled: 'true',
  })
  if (input.app === 'opencode') params.set('apiFormat', 'openai_chat')
  for (const [key, model] of Object.entries(input.models)) {
    if (model) params.set(key, model)
  }

  if (input.usage && supportsCCSwitchUsage(input.gateway)) {
    params.set('usageBaseUrl', base)
    params.set('usageScript', encodeUsageScript(buildUsageScript(input.usage)))
    params.set('usageEnabled', 'true')
    params.set('usageAutoInterval', '30')
  }

  return `ccswitch://v1/import?${params.toString()}`
}
