import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildExpirationMonthOptions,
  buildExpirationYearOptions,
  setVisibleMonthYear,
} from '../taskFormExpirationCalendar.ts'

test('buildExpirationYearOptions returns centered year range around visible month', () => {
  const visibleMonth = new Date(2026, 2, 1, 12, 0, 0, 0)

  const options = buildExpirationYearOptions(visibleMonth)

  assert.deepEqual(options, [2021, 2022, 2023, 2024, 2025, 2026, 2027, 2028, 2029, 2030, 2031])
})

test('buildExpirationMonthOptions returns all months from january to december', () => {
  const options = buildExpirationMonthOptions()

  assert.equal(options.length, 12)
  assert.equal(options[0], 1)
  assert.equal(options[11], 12)
})

test('setVisibleMonthYear keeps month when only year changes', () => {
  const visibleMonth = new Date(2026, 2, 1, 12, 0, 0, 0)

  const nextMonth = setVisibleMonthYear(visibleMonth, 2028)

  assert.equal(nextMonth.getFullYear(), 2028)
  assert.equal(nextMonth.getMonth(), 2)
  assert.equal(nextMonth.getDate(), 1)
})

test('setVisibleMonthYear keeps year when only month changes', () => {
  const visibleMonth = new Date(2026, 2, 1, 12, 0, 0, 0)

  const nextMonth = setVisibleMonthYear(visibleMonth, undefined, 11)

  assert.equal(nextMonth.getFullYear(), 2026)
  assert.equal(nextMonth.getMonth(), 10)
  assert.equal(nextMonth.getDate(), 1)
})
