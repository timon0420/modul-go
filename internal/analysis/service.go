package analysis

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// AnalyzeUser calculates report data only. Limit notifications are owned by Spring Boot.
func (s *Service) AnalyzeUser(ctx context.Context, login string) (AnalysisReport, error) {
	user, err := s.repo.FindUser(ctx, login)
	if err != nil {
		return AnalysisReport{}, err
	}

	location := applicationLocation()
	now := time.Now().In(location)
	year, month, day := now.Date()
	summary := make(map[string]int)
	totalDuration := 0
	for _, activity := range user.Activities {
		activityTime, ok := parseActivityDate(activity.Date, location)
		if !ok {
			slog.Warn("skipping activity with unsupported date", "type", "unknown")
			continue
		}
		activityYear, activityMonth, activityDay := activityTime.Date()
		if activityYear != year || activityMonth != month || activityDay != day {
			continue
		}
		duration := parseDuration(activity.Time)
		summary[strings.ToLower(activity.Name)] += duration
		totalDuration += duration
	}

	exceeded := []string{}
	globalLimit := "Brak"
	if user.DailyLimits != nil {
		if user.DailyLimits.GlobalLimit != nil {
			limit := *user.DailyLimits.GlobalLimit
			globalLimit = strconv.Itoa(limit) + " min"
			if totalDuration > limit {
				exceeded = append(exceeded, "GLOBAL")
			}
		}
		for _, limit := range user.DailyLimits.Activities {
			if summary[strings.ToLower(limit.ActivityName)] > limit.Limit {
				exceeded = append(exceeded, limit.ActivityName)
			}
		}
	}

	report := AnalysisReport{
		Login: login, Date: now.Format("2006-01-02"), DailyLimits: user.DailyLimits,
		Activities: user.Activities, Notifications: user.Notifications, GlobalLimit: globalLimit,
		TotalDuration: totalDuration, GlobalLimitExceeded: contains(exceeded, "GLOBAL"),
		ActivitiesSummary: summary, ExceededLimits: exceeded,
	}
	s.repo.WriteReport(report)
	return report, nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func applicationLocation() *time.Location {
	name := os.Getenv("APP_TIMEZONE")
	if name == "" {
		name = "Europe/Warsaw"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("invalid APP_TIMEZONE, using UTC", "time_zone", name, "error", err)
		return time.UTC
	}
	return location
}

func (s *Service) ListAll(ctx context.Context) ([]UserActivity, error) { return s.repo.ListAll(ctx) }
