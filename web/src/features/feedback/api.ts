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
import { v4 as createUUID } from 'uuid'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

export const FEEDBACK_PAGE_SIZE = 25
const EVENT_PAGE_SIZE = 50
const feedbackPath = '/api/arca/feedback'
export const feedbackRequestIdPattern = /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/

export const feedbackCategories = [
  'bug',
  'model',
  'access',
  'suggestion',
  'other',
] as const
export type FeedbackCategory = (typeof feedbackCategories)[number]
export type FeedbackStatus = 'open' | 'resolved' | 'closed'

type Envelope<T> = { success: boolean; message?: string; data: T }

export type FeedbackIssue = {
  id: string
  number: number
  owner_id?: string | null
  owner_username?: string | null
  title: string
  description?: string
  category: FeedbackCategory
  status: FeedbackStatus
  version: number
  activity_seq: number
  pinned_at?: number | null
  created_at: number
  updated_at: number
}

export type FeedbackEvent = {
  seq: number
  actor_id?: string | null
  actor_username?: string | null
  kind: string
  content?: string | null
  deleted_at?: number | null
  to_status?: FeedbackStatus | null
  created_at: number
}

export type FeedbackDetail = {
  issue: FeedbackIssue
  events: FeedbackEvent[]
  next_event_cursor: number
  has_more: boolean
}

export type FeedbackDraft = {
  title: string
  description: string
  category: FeedbackCategory
  request_id?: string
  include_diagnostics?: boolean
}

export type FeedbackCommand =
  | { action: 'reply'; content: string }
  | { action: 'edit_issue'; title: string; description: string }
  | { action: 'delete_issue' }
  | { action: 'set_pinned'; pinned: boolean }
  | { action: 'edit_reply'; seq: number; content: string }
  | { action: 'delete_reply'; seq: number }
  | { action: 'set_status'; status: FeedbackStatus; comment?: string }

type FeedbackMutation = { issue: FeedbackIssue }

export type FeedbackNotice = {
  id: number
  issue_id: string
  issue_title: string
  event_seq: number
  actor_id?: string | null
  actor_username?: string | null
  snippet: string
  created_at: number
  read_at?: number | null
}

export type FeedbackInbox = { items: FeedbackNotice[]; unread_count: number }
export type MarkRead = { all: true } | { ids: number[] }

export const feedbackKeys = {
  list: (session: string) => ['feedback', session, 'list'] as const,
  topic: (session: string, topic: string) =>
    ['feedback', session, 'topic', topic] as const,
  inbox: (session: string | null) => ['feedback', session, 'inbox'] as const,
  avatars: (session: string | null, ids: string[]) =>
    ['feedback', session, 'avatars', ids] as const,
}

export function useFeedbackSession() {
  return useAuthStore((state) => {
    const { user, accessToken, session } = state.auth
    return user && accessToken ? (session?.sid ?? null) : null
  })
}

async function read<T>(
  path: string,
  params?: Record<string, unknown>,
  signal?: AbortSignal
): Promise<T> {
  const response = await api.get<Envelope<T>>(path, {
    params,
    signal,
    skipErrorHandler: true,
    disableDuplicate: true,
  })
  return requireServerSuccess(response.data).data
}

async function write<T>(
  session: string,
  path: string,
  body: object
): Promise<T> {
  const response = await api.post<Envelope<T>>(path, body, {
    requestSessionID: session,
    skipErrorHandler: true,
  })
  return requireServerSuccess(response.data).data
}

// Keep the same idempotency key when retrying an unchanged operation after an
// uncertain response. The server rejects reuse of the key with a different payload.
export type FeedbackAttempt = { fingerprint: string; id: string }

export function feedbackAttempt(
  payload: object,
  previous: FeedbackAttempt | null
): FeedbackAttempt {
  const fingerprint = JSON.stringify(payload)
  if (previous?.fingerprint === fingerprint) return previous
  return { fingerprint, id: createUUID() }
}

export function listFeedback(offset: number, signal?: AbortSignal) {
  return read<{ items: FeedbackIssue[]; total: number }>(
    feedbackPath,
    { limit: FEEDBACK_PAGE_SIZE, skip: offset },
    signal
  )
}

export function getFeedback(
  topic: string,
  cursor: number,
  signal?: AbortSignal
) {
  return read<FeedbackDetail>(
    `${feedbackPath}/${encodeURIComponent(topic)}`,
    { limit: EVENT_PAGE_SIZE, after_event: cursor },
    signal
  )
}

export function createFeedback(
  session: string,
  draft: FeedbackDraft,
  attempt: FeedbackAttempt
) {
  return write<FeedbackMutation>(session, feedbackPath, {
    ...draft,
    client_request_id: attempt.id,
  })
}

export function applyFeedbackCommand(
  session: string,
  topic: FeedbackIssue,
  command: FeedbackCommand,
  attempt: FeedbackAttempt
) {
  return write<FeedbackMutation>(
    session,
    `${feedbackPath}/${encodeURIComponent(topic.id)}/commands`,
    {
      ...command,
      expected_version: topic.version,
      client_request_id: attempt.id,
    }
  )
}

export function getFeedbackDiagnostic(request: string) {
  return read<{ summary: string }>(
    `/api/arca/requests/${encodeURIComponent(request)}/diagnostics`
  )
}

export function getFeedbackInbox(signal?: AbortSignal) {
  return read<FeedbackInbox>(
    `${feedbackPath}/notifications`,
    { limit: 20 },
    signal
  )
}

export function markFeedbackRead(session: string, input: MarkRead) {
  return write<{ unread_count: number }>(
    session,
    `${feedbackPath}/notifications/read`,
    input
  )
}

export function getFeedbackAvatars(ids: string[], signal?: AbortSignal) {
  return read<Record<string, string>>(
    '/api/arca/avatars',
    { ids: ids.join(',') },
    signal
  )
}

export function feedbackActorIDs(ids: (string | null | undefined)[]): string[] {
  return [
    ...new Set(
      ids.filter((id): id is string => Boolean(id && /^\d+$/.test(id)))
    ),
  ]
    .sort()
    .slice(0, 100)
}
