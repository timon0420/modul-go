package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Models mirroring Spring Boot MongoDB models
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
	Start_time  string      `bson:"start_time" json:"start_time"`
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
	Login               string            `json:"login"`
	Date                string            `json:"date"`
	GlobalLimit         string            `json:"global_limit"`
	TotalDuration       int               `json:"total_duration_minutes"`
	GlobalLimitExceeded bool              `json:"global_limit_exceeded"`
	ActivitiesSummary   map[string]int    `json:"activities_summary_minutes"`
	ExceededLimits      []string          `json:"exceeded_limits"`
	NewNotifications    []Notification    `json:"new_notifications"`
}

// WebSocket connection map
var (
	clients   = make(map[string]*websocket.Conn)
	clientsMu sync.Mutex
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for the sandbox
		},
	}
)

func databaseConnection() *mongo.Collection {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("Warning: Error loading .env file, using environment variables")
	}

	MONGO_URI := os.Getenv("MONGO_URI")
	if MONGO_URI == "" {
		MONGO_URI = "mongodb+srv://medelaszymon_db_user:mKZFtqw5DQ2LvyzU@data.sawohxk.mongodb.net/"
	}

	clientOptions := options.Client().ApplyURI(MONGO_URI)
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	collection := client.Database("digital-activities").Collection("activities")
	return collection
}

// WebSocket handler
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	login := r.URL.Query().Get("login")
	if login == "" {
		http.Error(w, "Missing login parameter", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket Upgrade error: %v", err)
		return
	}

	clientsMu.Lock()
	if oldConn, exists := clients[login]; exists {
		oldConn.Close()
	}
	clients[login] = conn
	clientsMu.Unlock()

	log.Printf("User WebSocket connected: %s", login)

	defer func() {
		clientsMu.Lock()
		if clients[login] == conn {
			delete(clients, login)
		}
		clientsMu.Unlock()
		conn.Close()
		log.Printf("User WebSocket disconnected: %s", login)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// REST handler to run analysis
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	login := r.URL.Query().Get("login")
	if login == "" {
		http.Error(w, "Missing login parameter", http.StatusBadRequest)
		return
	}

	collection := databaseConnection()
	ctx := context.Background()

	// 1. Fetch user document from MongoDB
	var userUA UserActivity
	err := collection.FindOne(ctx, bson.M{"login": login}).Decode(&userUA)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// 2. Perform analysis on activities from today
	now := time.Now()
	todayYear, todayMonth, todayDay := now.Date()

	activitiesSummary := make(map[string]int)
	totalDuration := 0

	for _, act := range userUA.Activities {
		var actTime time.Time
		var parseErr error

		// Parse date (supports BSON primitive.DateTime or String)
		switch v := act.Date.(type) {
		case primitive.DateTime:
			actTime = v.Time()
		case string:
			actTime, parseErr = time.Parse("2006-01-02", v)
			if parseErr != nil {
				// Try parsing other standard formats if necessary
				actTime, parseErr = time.Parse(time.RFC3339, v)
			}
		default:
			log.Printf("Unknown date type: %T for activity %s", v, act.Name)
			continue
		}

		if parseErr != nil {
			log.Printf("Failed to parse date: %v", parseErr)
			continue
		}

		// Check if it belongs to today
		actYear, actMonth, actDay := actTime.Date()
		if actYear == todayYear && actMonth == todayMonth && actDay == todayDay {
			// Parse duration time (supports number types and strings)
			duration := 0
			switch t := act.Time.(type) {
			case string:
				duration, _ = strconv.Atoi(t)
			case int32:
				duration = int(t)
			case int64:
				duration = int(t)
			case float64:
				duration = int(t)
			case int:
				duration = t
			default:
				log.Printf("Unknown time type: %T", t)
			}

			activitiesSummary[strings.ToLower(act.Name)] += duration
			totalDuration += duration
		}
	}

	// 3. Compare with limits and prepare warnings
	var newNotifications []Notification
	exceededLimits := []string{}
	globalLimitStr := "Brak"

	if userUA.DailyLimits != nil {
		// Global limit check
		if userUA.DailyLimits.GlobalLimit != nil {
			gLimit := *userUA.DailyLimits.GlobalLimit
			globalLimitStr = strconv.Itoa(gLimit) + " min"
			if totalDuration > gLimit {
				msg := fmt.Sprintf("Przekroczono dobowy limit globalny aktywności! Czas: %d min, limit: %d min.", totalDuration, gLimit)
				exceededLimits = append(exceededLimits, "GLOBAL")
				
				// Avoid spam: check if same warning exists for today
				if !hasNotificationToday(userUA.Notifications, msg, todayYear, todayMonth, todayDay) {
					notif := Notification{
						ID:        primitive.NewObjectID().Hex(),
						Type:      "LIMIT_WARNING",
						Message:   msg,
						CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
						Read:      false,
					}
					newNotifications = append(newNotifications, notif)
				}
			}
		}

		// Individual limits check
		for _, lim := range userUA.DailyLimits.Activities {
			spent, ok := activitiesSummary[strings.ToLower(lim.ActivityName)]
			if ok && spent > lim.Limit {
				msg := fmt.Sprintf("Przekroczono dobowy limit dla aktywności '%s'! Czas: %d min, limit: %d min.", lim.ActivityName, spent, lim.Limit)
				exceededLimits = append(exceededLimits, lim.ActivityName)

				if !hasNotificationToday(userUA.Notifications, msg, todayYear, todayMonth, todayDay) {
					notif := Notification{
						ID:        primitive.NewObjectID().Hex(),
						Type:      "LIMIT_WARNING",
						Message:   msg,
						CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
						Read:      false,
					}
					newNotifications = append(newNotifications, notif)
				}
			}
		}
	}

	// 4. Save new notifications in MongoDB
	if len(newNotifications) > 0 {
		for _, notif := range newNotifications {
			_, err = collection.UpdateOne(
				ctx,
				bson.M{"login": login},
				bson.M{"$push": bson.M{"notifications": notif}},
			)
			if err != nil {
				log.Printf("Failed to save notification: %v", err)
			}
		}

		// 5. Send notifications via WebSockets
		clientsMu.Lock()
		conn, exists := clients[login]
		clientsMu.Unlock()
		if exists {
			for _, notif := range newNotifications {
				err := conn.WriteJSON(notif)
				if err != nil {
					log.Printf("WebSocket send error for user %s: %v", login, err)
				}
			}
		}
	}

	// 6. Generate analysis report
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

	// Save report to file (as in legacy implementation)
	reportJSON, err := json.MarshalIndent(report, "", "\t")
	if err == nil {
		_ = os.WriteFile("report.json", reportJSON, 0644)
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// Helper function to check if notification already exists for today
func hasNotificationToday(notifications []Notification, message string, year int, month time.Month, day int) bool {
	for _, n := range notifications {
		if n.Message == message {
			t := n.CreatedAt.Time()
			nY, nM, nD := t.Date()
			if nY == year && nM == month && nD == day {
				return true
			}
		}
	}
	return false
}

// Legacy report compatibility
func handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	collection := databaseConnection()
	cursor, err := collection.Find(context.Background(), bson.D{})
	if err != nil {
		log.Fatal(err)
	}
	defer cursor.Close(context.Background())

	var results []bson.M
	if err = cursor.All(context.Background(), &results); err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)

	jsonReport, err := json.MarshalIndent(results, "", "	")
	if err == nil {
		_ = os.WriteFile("report.json", jsonReport, 0644)
	}
}

func main() {
	http.HandleFunc("/report", handleReport)
	http.HandleFunc("/analyze", handleAnalyze)
	http.HandleFunc("/ws", handleWebSocket)

	log.Println("Starting analytical Go microservice on port :8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}