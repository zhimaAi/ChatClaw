package bootstrap

import (
	"chatclaw/internal/eino/tools"
	"chatclaw/internal/services/agents"
	"chatclaw/internal/services/scheduledtasks"

	einotool "github.com/cloudwego/eino/components/tool"
)

func newScheduledTaskManagementTools(agentsService *agents.AgentsService, scheduledTasksService *scheduledtasks.ScheduledTasksService) ([]einotool.BaseTool, error) {
	return tools.NewScheduledTaskManagementTools(&tools.ScheduledTaskManagementConfig{
		ListAgentsForMatchingFn: func() ([]tools.ScheduledTaskAgent, error) {
			items, err := agentsService.ListAgentsForMatching()
			if err != nil {
				return nil, err
			}
			out := make([]tools.ScheduledTaskAgent, 0, len(items))
			for _, item := range items {
				out = append(out, tools.ScheduledTaskAgent{
					ID:   item.ID,
					Name: item.Name,
				})
			}
			return out, nil
		},
		MatchAgentsByNameFn: func(query string) ([]tools.ScheduledTaskAgent, string, error) {
			items, status, err := agentsService.MatchAgentsByName(query)
			if err != nil {
				return nil, "", err
			}
			out := make([]tools.ScheduledTaskAgent, 0, len(items))
			for _, item := range items {
				out = append(out, tools.ScheduledTaskAgent{
					ID:   item.ID,
					Name: item.Name,
				})
			}
			return out, status, nil
		},
		ListScheduledTasksFn: func() ([]tools.ScheduledTaskRecord, error) {
			items, err := scheduledTasksService.ListScheduledTasks()
			if err != nil {
				return nil, err
			}
			return convertScheduledTaskRecords(items), nil
		},
		GetScheduledTaskByIDFn: func(id int64) (*tools.ScheduledTaskRecord, error) {
			item, err := scheduledTasksService.GetScheduledTaskByID(id)
			if err != nil {
				return nil, err
			}
			record := convertScheduledTaskRecord(*item)
			return &record, nil
		},
		FindScheduledTasksFn: func(name string) ([]tools.ScheduledTaskRecord, error) {
			items, err := scheduledTasksService.FindScheduledTasksByName(name)
			if err != nil {
				return nil, err
			}
			return convertScheduledTaskRecords(items), nil
		},
		ValidateScheduleFn: func(scheduleType, scheduleValue, cronExpr string) (*tools.ScheduledTaskValidationResult, error) {
			result, err := scheduledTasksService.ValidateSchedule(scheduleType, scheduleValue, cronExpr)
			if err != nil {
				return nil, err
			}
			return &tools.ScheduledTaskValidationResult{
				ScheduleType:  result.ScheduleType,
				ScheduleValue: result.ScheduleValue,
				CronExpr:      result.CronExpr,
				Timezone:      result.Timezone,
				NextRunAt:     result.NextRunAt,
			}, nil
		},
		CreateScheduledTaskFn: func(input tools.ScheduledTaskCreateInput) (*tools.ScheduledTaskRecord, error) {
			created, err := scheduledTasksService.CreateScheduledTask(scheduledtasks.CreateScheduledTaskInput{
				Name:          input.Name,
				Prompt:        input.Prompt,
				AgentID:       input.AgentID,
				ScheduleType:  input.ScheduleType,
				ScheduleValue: input.ScheduleValue,
				CronExpr:      input.CronExpr,
				Enabled:       input.Enabled,
			})
			if err != nil {
				return nil, err
			}
			record := convertScheduledTaskRecord(*created)
			return &record, nil
		},
		DeleteScheduledTaskFn: func(id int64) error {
			return scheduledTasksService.DeleteScheduledTask(id)
		},
		SetScheduledTaskFn: func(id int64, enabled bool) (*tools.ScheduledTaskRecord, error) {
			updated, err := scheduledTasksService.SetScheduledTaskEnabled(id, enabled)
			if err != nil {
				return nil, err
			}
			record := convertScheduledTaskRecord(*updated)
			return &record, nil
		},
	})
}

func convertScheduledTaskRecords(items []scheduledtasks.ScheduledTask) []tools.ScheduledTaskRecord {
	out := make([]tools.ScheduledTaskRecord, 0, len(items))
	for _, item := range items {
		out = append(out, convertScheduledTaskRecord(item))
	}
	return out
}

func convertScheduledTaskRecord(item scheduledtasks.ScheduledTask) tools.ScheduledTaskRecord {
	return tools.ScheduledTaskRecord{
		ID:            item.ID,
		Name:          item.Name,
		Prompt:        item.Prompt,
		AgentID:       item.AgentID,
		ScheduleType:  item.ScheduleType,
		ScheduleValue: item.ScheduleValue,
		CronExpr:      item.CronExpr,
		Timezone:      item.Timezone,
		Enabled:       item.Enabled,
		LastRunAt:     item.LastRunAt,
		NextRunAt:     item.NextRunAt,
		LastStatus:    item.LastStatus,
		LastError:     item.LastError,
		LastRunID:     item.LastRunID,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}
