package scheduledtasks

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type scheduler struct {
	mu      sync.Mutex
	parser  cron.Parser
	entries map[int64]cron.EntryID
	cron    *cron.Cron
}

func newScheduler() *scheduler {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	return &scheduler{
		parser:  parser,
		entries: make(map[int64]cron.EntryID),
		cron:    cron.New(cron.WithParser(parser)),
	}
}

func (s *scheduler) start() {
	s.cron.Start()
}

func (s *scheduler) stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *scheduler) register(task ScheduledTask, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldID, ok := s.entries[task.ID]; ok {
		s.cron.Remove(oldID)
		delete(s.entries, task.ID)
	}
	if !task.Enabled {
		return nil
	}

	entryID, err := s.cron.AddFunc(task.CronExpr, fn)
	if err != nil {
		return fmt.Errorf("register cron: %w", err)
	}
	s.entries[task.ID] = entryID
	return nil
}

func (s *scheduler) unregister(taskID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[taskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, taskID)
	}
}

func (s *scheduler) next(expr string, now time.Time) (*time.Time, error) {
	schedule, err := s.parser.Parse(expr)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(now)
	return &next, nil
}

func buildScheduleDefinition(scheduleType, scheduleValue, expr string, now time.Time) (scheduleDefinition, error) {
	s := newScheduler()
	next, err := s.next(expr, now)
	if err != nil {
		return scheduleDefinition{}, err
	}
	return scheduleDefinition{
		ScheduleType:  scheduleType,
		ScheduleValue: scheduleValue,
		CronExpr:      expr,
		Timezone:      now.Location().String(),
		NextRunAt:     next,
	}, nil
}
