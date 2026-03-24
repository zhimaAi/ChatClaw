import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildOperationLogDisplayRows,
  formatOperationLogFieldValue,
} from '../operationLogTable.ts'

test('formatOperationLogFieldValue supports injected localized schedule formatter', () => {
  const result = formatOperationLogFieldValue(
    'schedule_time',
    '{"hour":9,"minute":0,"day_of_month":4}',
    {
      monthly: ({ day, time }) => `Day ${day} ${time}`,
      weekly: ({ labels, time }) => `Weekly ${labels} ${time}`,
      daily: ({ time }) => `Daily ${time}`,
      interval: ({ value }) => `Every ${value} minutes`,
      weekdayLabel: (value) => ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'][value] ?? String(value),
    }
  )

  assert.equal(result, 'Day 4 09:00')
})

test('formatOperationLogFieldValue formats monthly custom schedule JSON', () => {
  const result = formatOperationLogFieldValue(
    'schedule_time',
    '{"hour":9,"minute":0,"day_of_month":4}'
  )

  assert.equal(result, '每月 4 号 09:00')
})

test('formatOperationLogFieldValue formats weekly custom schedule JSON', () => {
  const result = formatOperationLogFieldValue(
    'schedule_time',
    '{"hour":9,"minute":0,"weekdays":[1,3,5]}'
  )

  assert.equal(result, '每周一 周三 周五 09:00')
})

test('buildOperationLogDisplayRows expands one log into one row per changed field', () => {
  const rows = buildOperationLogDisplayRows([
    {
      id: 7,
      task_name_snapshot: '天气预报',
      operation_type: 'update',
      operation_source: 'manual',
      created_at: '2026-03-23T08:00:00Z',
      changed_fields: [
        {
          field_key: 'prompt',
          field_label: '提示词',
          before: '旧提示词',
          after: '新提示词',
        },
        {
          field_key: 'schedule_time',
          field_label: '执行时间',
          before: '{"hour":9,"minute":0}',
          after: '{"hour":10,"minute":30}',
        },
      ],
    },
  ])

  assert.equal(rows.length, 2)
  assert.equal(rows[0].taskName, '天气预报')
  assert.equal(rows[0].fieldLabel, '提示词')
  assert.equal(rows[1].taskName, '')
  assert.equal(rows[1].fieldLabel, '执行时间')
  assert.equal(rows[1].beforeValue, '每天 09:00')
  assert.equal(rows[1].afterValue, '每天 10:30')
  assert.equal(rows[0].showSharedColumns, true)
  assert.equal(rows[1].showSharedColumns, false)
})
