package analysis

import (
	"context"
	"encoding/json"
	"os"
	"time"

	appmongo "connect-to-mongodb/mongo"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

type Repository struct {
	client     *mongodriver.Client
	collection *mongodriver.Collection
}

func NewRepository(ctx context.Context) (*Repository, error) {
	client, collection, err := appmongo.Connect(ctx, appmongo.Config{})
	if err != nil {
		return nil, err
	}

	return &Repository{client: client, collection: collection}, nil
}

func (r *Repository) Close(ctx context.Context) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Disconnect(ctx)
}

func (r *Repository) FindUser(ctx context.Context, login string) (*UserActivity, error) {
	var user UserActivity
	if err := r.collection.FindOne(ctx, bson.M{"login": login}).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) ListAll(ctx context.Context) ([]UserActivity, error) {
	cursor, err := r.collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []UserActivity
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) SaveNotifications(ctx context.Context, login string, notifications []Notification) error {
	for _, notif := range notifications {
		_, err := r.collection.UpdateOne(
			ctx,
			bson.M{"login": login},
			bson.M{"$push": bson.M{"notifications": notif}},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) WriteReport(report AnalysisReport) {
	reportJSON, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return
	}
	_ = os.WriteFile("report.json", reportJSON, 0644)
}

func parseActivityDate(value interface{}) (time.Time, bool) {
	switch v := value.(type) {
	case primitive.DateTime:
		return v.Time(), true
	case string:
		if parsed, err := time.Parse("2006-01-02", v); err == nil {
			return parsed, true
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			return parsed, true
		}
	case time.Time:
		return v, true
	}
	return time.Time{}, false
}

func parseDuration(value interface{}) int {
	switch v := value.(type) {
	case string:
		if parsed, err := time.ParseDuration(v); err == nil {
			return int(parsed.Minutes())
		}
		return 0
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func hasNotificationToday(notifications []Notification, message string, year int, month time.Month, day int) bool {
	for _, n := range notifications {
		if n.Message != message {
			continue
		}
		t := n.CreatedAt.Time()
		nYear, nMonth, nDay := t.Date()
		if nYear == year && nMonth == month && nDay == day {
			return true
		}
	}
	return false
}
