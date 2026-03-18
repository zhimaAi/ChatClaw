import type { Agent, ScheduledTask } from '../types'

type TaskTableDisplayTask = Pick<ScheduledTask, 'agent_id' | 'last_run_at'>
type TaskTableDisplayAgent = Pick<Agent, 'id' | 'name'>

export function buildTaskTableDisplay(task: TaskTableDisplayTask, agents: TaskTableDisplayAgent[]) {
  const matchedAgent = agents.find((agent) => agent.id === task.agent_id)

  return {
    agent: {
      name: matchedAgent?.name || '-',
      showNextRun: false,
    },
    schedule: {
      showLastRun: !!task.last_run_at,
    },
  }
}
