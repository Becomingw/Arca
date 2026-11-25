import type { MessageKey } from './copy'

export const OPERATIONS_BEAT_MS = 1200
export const operationSteps = [
  { id: 'report', icon: 'message', title: 'site.home.operations.report', description: 'site.home.operations.reportDescription', result: 'site.home.operations.reportResult', tools: [
    { name: 'arca_feedback_list', label: 'site.home.operations.findIssue' },
    { name: 'arca_feedback_get', label: 'site.home.operations.readIssue' },
    { name: 'arca_identity', label: 'site.home.operations.checkIdentity' },
  ] },
  { id: 'diagnose', icon: 'search', title: 'site.home.operations.diagnose', description: 'site.home.operations.diagnoseDescription', result: 'site.home.operations.diagnoseResult', tools: [
    { name: 'arca_feedback_get', label: 'site.home.operations.readIssue' },
    { name: 'arca_request_diagnostics', label: 'site.home.operations.checkRequest' },
    { name: 'arca_providers_list', label: 'site.home.operations.checkProvider' },
  ] },
  { id: 'repair', icon: 'wrench', title: 'site.home.operations.repair', description: 'site.home.operations.repairDescription', result: 'site.home.operations.repairResult', tools: [
    { name: 'arca_feedback_command', label: 'site.home.operations.previewChange' },
    { name: 'arca_feedback_command', label: 'site.home.operations.confirmChange' },
    { name: 'arca_feedback_get', label: 'site.home.operations.readAudit' },
  ] },
  { id: 'resolve', icon: 'check', title: 'site.home.operations.resolve', description: 'site.home.operations.resolveDescription', result: 'site.home.operations.resolveResult', tools: [
    { name: 'arca_request_diagnostics', label: 'site.home.operations.verifyResult' },
    { name: 'arca_feedback_command', label: 'site.home.operations.replyIssue' },
    { name: 'arca_feedback_command', label: 'site.home.operations.resolveIssue' },
  ] },
] as const satisfies ReadonlyArray<{
  id: string
  icon: string
  title: MessageKey
  description: MessageKey
  result: MessageKey
  tools: ReadonlyArray<{ name: string, label: MessageKey }>
}>
