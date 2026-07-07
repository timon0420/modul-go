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
	"go.mongodb.org/mongo-driver/mongo/options"
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

func (r *Repository) ImportUsers(ctx context.Context, users []map[string]interface{}) (int, error) {
	imported := 0
	for _, user := range users {
		loginValue, ok := user["login"].(string)
		if !ok || loginValue == "" {
			continue
		}
		delete(user, "_id")
		delete(user, "id")
		if _, ok := user["activities"]; !ok || user["activities"] == nil {
			user["activities"] = []interface{}{}
		}
		if _, ok := user["notifications"]; !ok || user["notifications"] == nil {
			user["notifications"] = []interface{}{}
		}
		_, err := r.collection.ReplaceOne(
			ctx,
			bson.M{"login": loginValue},
			user,
			options.Replace().SetUpsert(true),
		)
		if err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func (r *Repository) DeleteAll(ctx context.Context) (int64, error) {
	result, err := r.collection.DeleteMany(ctx, bson.D{})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

func (r *Repository) DeleteByLogin(ctx context.Context, login string) (int64, error) {
	result, err := r.collection.DeleteOne(ctx, bson.M{"login": login})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

func (r *Repository) WriteReport(report AnalysisReport) {
	reportJSON, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return
	}
	_ = os.WriteFile("report.json", reportJSON, 0644)
}

func parseActivityDate(value interface{}, location *time.Location) (time.Time, bool) {
	switch v := value.(type) {
	case primitive.DateTime:
		return v.Time().In(location), true
	case string:
		if parsed, err := time.ParseInLocation("2006-01-02", v, location); err == nil {
			return parsed, true
		}
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			return parsed.In(location), true
		}
	case time.Time:
		return v.In(location), true
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
