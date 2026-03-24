import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildOperationLogDisplayRows,
  formatOperationLogFieldValue,
} from '../operationLogTable.ts'
import { createScheduleTextFormatter, getOperationLogFieldLabel } from '../scheduledTaskText.ts'

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

test('formatOperationLogFieldValue only localizes notification channel platform', () => {
  const zhFormatter = createScheduleTextFormatter((key) => {
    const messages = {
      'channels.platforms.feishu': '飞书',
    }
    return messages[key] ?? key
  })
  const enFormatter = createScheduleTextFormatter((key) => {
    const messages = {
      'channels.platforms.feishu': 'Feishu',
    }
    return messages[key] ?? key
  })

  assert.equal(
    formatOperationLogFieldValue('notification_channels', 'feishu: 按需', zhFormatter),
    '飞书: 按需'
  )
  assert.equal(
    formatOperationLogFieldValue('notification_channels', 'feishu: 按需', enFormatter),
    'Feishu: 按需'
  )
})

test('formatOperationLogFieldValue localizes status values from persisted chinese labels', () => {
  const enFormatter = createScheduleTextFormatter((key) => {
    const messages = {
      'scheduledTasks.operationLog.status.enabled': 'Enabled',
      'scheduledTasks.operationLog.status.disabled': 'Disabled',
    }
    return messages[key] ?? key
  })

  assert.equal(formatOperationLogFieldValue('status', '启用', enFormatter), 'Enabled')
  assert.equal(formatOperationLogFieldValue('status', '停用', enFormatter), 'Disabled')
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

test('buildOperationLogDisplayRows localizes changed field labels with field keys', () => {
  const translate = (key) => {
    const messages = {
      'scheduledTasks.operationLog.fields.prompt': 'Prompt',
      'scheduledTasks.operationLog.fields.scheduleTime': 'Schedule Time',
    }
    return messages[key] ?? key
  }

  const rows = buildOperationLogDisplayRows(
    [
      {
        id: 8,
        task_name_snapshot: 'Weather',
        operation_type: 'update',
        operation_source: 'manual',
        created_at: '2026-03-24T08:00:00Z',
        changed_fields: [
          {
            field_key: 'prompt',
            field_label: '提示词',
            before: 'old prompt',
            after: 'new prompt',
          },
          {
            field_key: 'schedule_time',
            field_label: '执行时间',
            before: '{"hour":9,"minute":0}',
            after: '{"hour":10,"minute":30}',
          },
        ],
      },
    ],
    createScheduleTextFormatter(translate),
    (fieldKey, fieldLabel) => getOperationLogFieldLabel(fieldKey, fieldLabel, translate)
  )

  assert.equal(rows[0].fieldLabel, 'Prompt')
  assert.equal(rows[1].fieldLabel, 'Schedule Time')
})

test('buildOperationLogDisplayRows renders placeholders for empty changed fields', () => {
  const rows = buildOperationLogDisplayRows([
    {
      id: 9,
      task_name_snapshot: 'Create task',
      operation_type: 'create',
      operation_source: 'manual',
      created_at: '2026-03-24T08:00:00Z',
      changed_fields: [],
    },
  ])

  assert.equal(rows.length, 1)
  assert.equal(rows[0].fieldLabel, '-')
  assert.equal(rows[0].beforeValue, '--')
  assert.equal(rows[0].afterValue, '--')
  assert.equal(rows[0].showSharedColumns, true)
})

test('getOperationLogFieldLabel falls back to persisted label for unknown fields', () => {
  assert.equal(getOperationLogFieldLabel('custom_field', '自定义字段'), '自定义字段')
})
