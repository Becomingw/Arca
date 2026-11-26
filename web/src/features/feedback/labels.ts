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
import type { TFunction } from 'i18next'

import type { FeedbackCategory, FeedbackStatus } from './api'

export function feedbackStatusLabel(t: TFunction, status: FeedbackStatus) {
  switch (status) {
    case 'open':
      return t('feedback.status.open', { defaultValue: 'Open' })
    case 'resolved':
      return t('feedback.status.resolved', { defaultValue: 'Resolved' })
    case 'closed':
      return t('feedback.status.closed', { defaultValue: 'Closed' })
  }
}

export function feedbackCategoryLabel(
  t: TFunction,
  category: FeedbackCategory
) {
  switch (category) {
    case 'bug':
      return t('feedback.category.bug', { defaultValue: 'Problem report' })
    case 'model':
      return t('feedback.category.model', { defaultValue: 'Model' })
    case 'access':
      return t('feedback.category.access', { defaultValue: 'Entitlements' })
    case 'suggestion':
      return t('feedback.category.suggestion', { defaultValue: 'Suggestion' })
    case 'other':
      return t('feedback.category.other', { defaultValue: 'Other' })
  }
}
