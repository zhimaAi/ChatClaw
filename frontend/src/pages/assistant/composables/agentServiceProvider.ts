import { inject, provide, type InjectionKey } from 'vue'
import {
  AgentsService,
  type Agent,
  type CreateAgentInput,
  type UpdateAgentInput,
  type FileEntry,
} from '@bindings/chatclaw/internal/services/agents'

export interface AgentServiceApi {
  ListAgents: () => Promise<Agent[]>
  GetAgent: (id: number) => Promise<Agent | null>
  CreateAgent: (input: CreateAgentInput) => Promise<Agent | null>
  UpdateAgent: (id: number, input: UpdateAgentInput) => Promise<Agent | null>
  DeleteAgent: (id: number) => Promise<void>
  GetDefaultPrompt: () => Promise<string>
  ReadIconFile: (path: string) => Promise<string>
  GetDefaultWorkDir: () => Promise<string>
  GetWorkspaceDir: (agentID: number, conversationID: number) => Promise<string>
  ListWorkspaceFiles: (agentID: number, conversationID: number) => Promise<FileEntry[]>
}

export const AgentServiceKey: InjectionKey<AgentServiceApi> = Symbol('AgentServiceApi')

const defaultAgentService: AgentServiceApi = {
  ListAgents: AgentsService.ListAgents,
  GetAgent: AgentsService.GetAgent,
  CreateAgent: AgentsService.CreateAgent as any,
  UpdateAgent: AgentsService.UpdateAgent as any,
  DeleteAgent: AgentsService.DeleteAgent,
  GetDefaultPrompt: AgentsService.GetDefaultPrompt,
  ReadIconFile: AgentsService.ReadIconFile,
  GetDefaultWorkDir: AgentsService.GetDefaultWorkDir,
  GetWorkspaceDir: AgentsService.GetWorkspaceDir,
  ListWorkspaceFiles: AgentsService.ListWorkspaceFiles,
}

export function provideAgentService(service?: AgentServiceApi) {
  provide(AgentServiceKey, service ?? defaultAgentService)
}

export function useAgentService(): AgentServiceApi {
  return inject(AgentServiceKey, defaultAgentService)
}
