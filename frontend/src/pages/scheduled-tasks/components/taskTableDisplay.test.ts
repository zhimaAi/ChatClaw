import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
// @ts-expect-error helper will be created after the failing test is observed.
import { buildTaskTableDisplay } from './taskTableDisplay.ts'

describe('buildTaskTableDisplay', () => {
  it('does not show next run text in the agent column', () => {
    const task = {
      id: 1,
      name: '日报',
      prompt: '生成日报',
      agent_id: 7,
      schedule_type: 'preset',
      schedule_value: 'every_day_0900',
      cron_expr: '0 9 * * *',
      timezone: 'Asia/Shanghai',
      enabled: true,
      last_run_at: new Date('2026-03-13T01:00:00.000Z'),
      next_run_at: new Date('2026-03-14T01:00:00.000Z'),
      last_status: 'success',
      last_error: '',
      last_run_id: 100,
      created_at: new Date('2026-03-12T01:00:00.000Z'),
      updated_at: new Date('2026-03-13T01:00:00.000Z'),
    }
    const agents = [{ id: 7, name: '运营助手' }]

    const display = buildTaskTableDisplay(task, agents)

    assert.equal(display.agent.name, '运营助手')
    assert.equal(display.agent.showNextRun, false)
  })

  it('hides last run text when the task has never run', () => {
    const task = {
      id: 2,
      name: '周报',
      prompt: '生成周报',
      agent_id: 8,
      schedule_type: 'preset',
      schedule_value: 'every_monday_0900',
      cron_expr: '0 9 * * 1',
      timezone: 'Asia/Shanghai',
      enabled: true,
      last_run_at: null,
      next_run_at: new Date('2026-03-16T01:00:00.000Z'),
      last_status: '',
      last_error: '',
      last_run_id: null,
      created_at: new Date('2026-03-12T01:00:00.000Z'),
      updated_at: new Date('2026-03-13T01:00:00.000Z'),
    }

    const display = buildTaskTableDisplay(task, [])

    assert.equal(display.schedule.showLastRun, false)
  })
})
