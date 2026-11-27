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
import { afterEach, describe, expect, it } from 'vitest'

import {
  DASHBOARD_CHART_PREFERENCES_STORAGE_KEY,
  TIME_GRANULARITY_STORAGE_KEY,
} from '../../constants'
import { getSavedChartPreferences } from '../filters'

afterEach(() => window.localStorage.clear())

describe('dashboard model chart preferences', () => {
  it('opens with a daily chart for the latest 14 days without saved preferences', () => {
    expect(getSavedChartPreferences()).toMatchObject({
      defaultTimeRangeDays: 14,
      defaultTimeGranularity: 'day',
    })
  })

  it('uses explicitly saved range and granularity', () => {
    window.localStorage.setItem(
      DASHBOARD_CHART_PREFERENCES_STORAGE_KEY,
      JSON.stringify({
        defaultTimeRangeDays: 29,
        defaultTimeGranularity: 'week',
      })
    )

    expect(getSavedChartPreferences()).toMatchObject({
      defaultTimeRangeDays: 29,
      defaultTimeGranularity: 'week',
    })
  })

  it('keeps the previously saved legacy granularity', () => {
    window.localStorage.setItem(TIME_GRANULARITY_STORAGE_KEY, 'hour')

    expect(getSavedChartPreferences().defaultTimeGranularity).toBe('hour')
  })
})
