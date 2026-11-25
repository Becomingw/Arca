export const HARNESS_ROTATION_MS = 4800

export const harnesses = [
  { id: 'codex', name: 'Codex', protocol: 'OpenAI Responses', logo: import.meta.env.BASE_URL + 'openai.svg', monochrome: true },
  { id: 'claude', name: 'Claude Code', protocol: 'Claude Messages', logo: import.meta.env.BASE_URL + 'claude-color.svg', monochrome: false },
  { id: 'opencode', name: 'OpenCode', protocol: 'OpenAI / Claude', logo: import.meta.env.BASE_URL + 'opencode.svg', monochrome: true },
  { id: 'gemini', name: 'Gemini CLI', protocol: 'Gemini API', logo: import.meta.env.BASE_URL + 'gemini-color.svg', monochrome: false },
  { id: 'cline', name: 'Cline', protocol: 'OpenAI Chat Completions', logo: import.meta.env.BASE_URL + 'cline.svg', monochrome: true },
] as const

export function wrapHarnessIndex(index: number): number {
  return ((index % harnesses.length) + harnesses.length) % harnesses.length
}
