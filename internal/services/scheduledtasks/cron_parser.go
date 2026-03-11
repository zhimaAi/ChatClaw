package scheduledtasks

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type scheduleDefinition struct {
	ScheduleType  string
	ScheduleValue string
	CronExpr      string
	Timezone      string
	NextRunAt     *time.Time
}

type customScheduleValue struct {
	Minute     int   `json:"minute"`
	Hour       int   `json:"hour"`
	Weekdays   []int `json:"weekdays,omitempty"`
	DayOfMonth *int  `json:"day_of_month,omitempty"`
}

func parseSchedule(scheduleType, scheduleValue, cronExpr string, now time.Time) (scheduleDefinition, error) {
	switch strings.TrimSpace(scheduleType) {
	case ScheduleTypePreset:
		return parsePresetSchedule(scheduleValue, now)
	case ScheduleTypeCustom:
		return parseCustomSchedule(scheduleValue, now)
	case ScheduleTypeCron:
		return parseCronSchedule(cronExpr, scheduleValue, now)
	default:
		return scheduleDefinition{}, fmt.Errorf("unsupported schedule type: %s", scheduleType)
	}
}

func parsePresetSchedule(scheduleValue string, now time.Time) (scheduleDefinition, error) {
	switch strings.TrimSpace(scheduleValue) {
	case "every_minute":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "* * * * *", now)
	case "every_5_minutes":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "*/5 * * * *", now)
	case "every_15_minutes":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "*/15 * * * *", now)
	case "every_hour":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 * * * *", now)
	case "every_day_0900":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 9 * * *", now)
	case "every_day_1800":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 18 * * *", now)
	case "weekdays_0900":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 9 * * 1-5", now)
	case "every_monday_0900":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 9 * * 1", now)
	case "every_month_1_0900":
		return buildScheduleDefinition(ScheduleTypePreset, scheduleValue, "0 9 1 * *", now)
	default:
		return scheduleDefinition{}, fmt.Errorf("unsupported preset schedule: %s", scheduleValue)
	}
}

func parseCustomSchedule(scheduleValue string, now time.Time) (scheduleDefinition, error) {
	var custom customScheduleValue
	if err := json.Unmarshal([]byte(scheduleValue), &custom); err != nil {
		return scheduleDefinition{}, fmt.Errorf("invalid custom schedule value: %w", err)
	}
	if custom.Minute < 0 || custom.Minute > 59 {
		return scheduleDefinition{}, fmt.Errorf("minute out of range")
	}
	if custom.Hour < 0 || custom.Hour > 23 {
		return scheduleDefinition{}, fmt.Errorf("hour out of range")
	}

	minute := strconv.Itoa(custom.Minute)
	hour := strconv.Itoa(custom.Hour)

	switch {
	case custom.DayOfMonth != nil:
		if *custom.DayOfMonth < 1 || *custom.DayOfMonth > 31 {
			return scheduleDefinition{}, fmt.Errorf("day_of_month out of range")
		}
		return buildScheduleDefinition(ScheduleTypeCustom, scheduleValue, fmt.Sprintf("%s %s %d * *", minute, hour, *custom.DayOfMonth), now)
	case len(custom.Weekdays) > 0:
		parts := make([]string, 0, len(custom.Weekdays))
		for _, weekday := range custom.Weekdays {
			if weekday < 0 || weekday > 6 {
				return scheduleDefinition{}, fmt.Errorf("weekday out of range")
			}
			parts = append(parts, strconv.Itoa(weekday))
		}
		return buildScheduleDefinition(ScheduleTypeCustom, scheduleValue, fmt.Sprintf("%s %s * * %s", minute, hour, strings.Join(parts, ",")), now)
	default:
		return buildScheduleDefinition(ScheduleTypeCustom, scheduleValue, fmt.Sprintf("%s %s * * *", minute, hour), now)
	}
}

func parseCronSchedule(cronExpr string, scheduleValue string, now time.Time) (scheduleDefinition, error) {
	expr := strings.TrimSpace(cronExpr)
	if expr == "" {
		expr = strings.TrimSpace(scheduleValue)
	}
	if expr == "" {
		return scheduleDefinition{}, fmt.Errorf("cron expression required")
	}
	return buildScheduleDefinition(ScheduleTypeCron, scheduleValue, expr, now)
}
