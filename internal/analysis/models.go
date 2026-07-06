package analysis

import "go.mongodb.org/mongo-driver/bson/primitive"

type ActivityLimit struct {
	ActivityName string `bson:"activity_name" json:"activity_name"`
	Limit        int    `bson:"limit" json:"limit"`
}

type DailyLimits struct {
	GlobalLimit *int            `bson:"global_limit,omitempty" json:"global_limit,omitempty"`
	Activities  []ActivityLimit `bson:"activities,omitempty" json:"activities,omitempty"`
}

type Notification struct {
	ID        string             `bson:"id" json:"id"`
	Type      string             `bson:"type" json:"type"`
	Message   string             `bson:"message" json:"message"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
	Read      bool               `bson:"read" json:"read"`
}

type Activity struct {
	ID          string      `bson:"_id" json:"id"`
	Name        string      `bson:"activity_name" json:"activity_name"`
	Description string      `bson:"activity_description" json:"activity_description"`
	StartTime   string      `bson:"start_time" json:"start_time"`
	Time        interface{} `bson:"time" json:"time"`
	Date        interface{} `bson:"date" json:"date"`
}

type UserActivity struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Login         string             `bson:"login" json:"login"`
	Activities    []Activity         `bson:"activities" json:"activities"`
	DailyLimits   *DailyLimits       `bson:"daily_limits,omitempty" json:"daily_limits,omitempty"`
	Notifications []Notification     `bson:"notifications,omitempty" json:"notifications,omitempty"`
}

type AnalysisReport struct {
	Login               string         `json:"login"`
	Date                string         `json:"date"`
	GlobalLimit         string         `json:"global_limit"`
	TotalDuration       int            `json:"total_duration_minutes"`
	GlobalLimitExceeded bool           `json:"global_limit_exceeded"`
	ActivitiesSummary   map[string]int `json:"activities_summary_minutes"`
	ExceededLimits      []string       `json:"exceeded_limits"`
}
