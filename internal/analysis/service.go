package analysis

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) AnalyzeUser(ctx context.Context, login string) (AnalysisReport, error) {
	userUA, err := s.repo.FindUser(ctx, login)
	if err != nil {
		return AnalysisReport{}, err
	}

	now := time.Now()
	todayYear, todayMonth, todayDay := now.Date()

	activitiesSummary := make(map[string]int)
	totalDuration := 0

	for _, act := range userUA.Activities {
		actTime, ok := parseActivityDate(act.Date)
		if !ok {
			log.Printf("Skipping activity with unsupported date type: %T", act.Date)
			continue
		}

		actYear, actMonth, actDay := actTime.Date()
		if actYear != todayYear || actMonth != todayMonth || actDay != todayDay {
			continue
		}

		duration := parseDuration(act.Time)
		activitiesSummary[strings.ToLower(act.Name)] += duration
		totalDuration += duration
	}

	var newNotifications []Notification
	exceededLimits := []string{}
	globalLimitStr := "Brak"

	if userUA.DailyLimits != nil {
		if userUA.DailyLimits.GlobalLimit != nil {
			gLimit := *userUA.DailyLimits.GlobalLimit
			globalLimitStr = strconv.Itoa(gLimit) + " min"
			if totalDuration > gLimit {
				msg := fmt.Sprintf("Przekroczono dobowy limit globalny aktywności! Czas: %d min, limit: %d min.", totalDuration, gLimit)
				exceededLimits = append(exceededLimits, "GLOBAL")
				if !hasNotificationToday(userUA.Notifications, msg, todayYear, todayMonth, todayDay) {
					newNotifications = append(newNotifications, Notification{
						ID:        primitive.NewObjectID().Hex(),
						Type:      "LIMIT_WARNING",
						Message:   msg,
						CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
						Read:      false,
					})
				}
			}
		}

		for _, lim := range userUA.DailyLimits.Activities {
			spent, ok := activitiesSummary[strings.ToLower(lim.ActivityName)]
			if !ok || spent <= lim.Limit {
				continue
			}

			msg := fmt.Sprintf("Przekroczono dobowy limit dla aktywności '%s'! Czas: %d min, limit: %d min.", lim.ActivityName, spent, lim.Limit)
			exceededLimits = append(exceededLimits, lim.ActivityName)
			if !hasNotificationToday(userUA.Notifications, msg, todayYear, todayMonth, todayDay) {
				newNotifications = append(newNotifications, Notification{
					ID:        primitive.NewObjectID().Hex(),
					Type:      "LIMIT_WARNING",
					Message:   msg,
					CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
					Read:      false,
				})
			}
		}
	}

	if len(newNotifications) > 0 {
		if err := s.repo.SaveNotifications(ctx, login, newNotifications); err != nil {
			return AnalysisReport{}, fmt.Errorf("save notifications: %w", err)
		}
	}

	report := AnalysisReport{
		Login:               login,
		Date:                now.Format("2006-01-02"),
		GlobalLimit:         globalLimitStr,
		TotalDuration:       totalDuration,
		GlobalLimitExceeded: len(exceededLimits) > 0 && exceededLimits[0] == "GLOBAL",
		ActivitiesSummary:   activitiesSummary,
		ExceededLimits:      exceededLimits,
		NewNotifications:    newNotifications,
	}

	s.repo.WriteReport(report)
	return report, nil
}

func (s *Service) ListAll(ctx context.Context) ([]UserActivity, error) {
	return s.repo.ListAll(ctx)
}
