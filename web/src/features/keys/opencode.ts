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
type OpenCodeConfig = {
  provider?: {
    aether?: { options?: { baseURL?: unknown }; models?: unknown }
  }
}

export async function fetchOpenCodeConfig(
  tokenKey: string,
  signal?: AbortSignal
): Promise<OpenCodeConfig> {
  // This endpoint requires the selected inference key, rather than the browser
  // session used by the dashboard client. Never put the key in a URL or cache.
  const response = await fetch('/api/opencode/config', {
    headers: { Authorization: `Bearer ${tokenKey}` },
    credentials: 'omit',
    cache: 'no-store',
    redirect: 'error',
    signal,
  })
  if (!response.ok) throw new Error('OpenCode configuration unavailable')
  return response.json()
}

export async function downloadOpenCodeConfig(tokenKey: string): Promise<void> {
  const config = await fetchOpenCodeConfig(tokenKey)
  if (!config.provider?.aether) {
    throw new Error('Invalid OpenCode configuration')
  }
  const file = new Blob([JSON.stringify(config, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = 'opencode.json'
  document.body.append(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
