package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	proto "connect-to-mongodb/grpc-analysis/proto"
	appanalysis "connect-to-mongodb/internal/analysis"
)

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

var (
	clients   = make(map[string]*wsClient)
	clientsMu sync.Mutex
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env not loaded, using environment variables")
	}

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = "127.0.0.1:50051"
	}

	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC server %s: %v", grpcAddr, err)
	}
	defer conn.Close()

	client := proto.NewAnalysisServiceClient(conn)

	reportRepo, err := appanalysis.NewRepository(context.Background())
	if err != nil {
		log.Printf("report endpoint will be unavailable: %v", err)
	}
	if reportRepo != nil {
		defer reportRepo.Close(context.Background())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		login := r.URL.Query().Get("login")
		if login == "" {
			http.Error(w, "missing login parameter", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("websocket upgrade error: %v", err)
			return
		}

		clientsMu.Lock()
		if oldConn, exists := clients[login]; exists {
			oldConn.mu.Lock()
			_ = oldConn.conn.Close()
			oldConn.mu.Unlock()
		}
		clients[login] = &wsClient{conn: conn}
		clientsMu.Unlock()

		log.Printf("websocket connected for user %s", login)

		defer func() {
			clientsMu.Lock()
			if client, exists := clients[login]; exists && client.conn == conn {
				delete(clients, login)
			}
			clientsMu.Unlock()
			_ = conn.Close()
			log.Printf("websocket disconnected for user %s", login)
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	})
	mux.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		login := r.URL.Query().Get("login")
		if login == "" {
			http.Error(w, "missing login parameter", http.StatusBadRequest)
			return
		}

		var beforeCount int
		if reportRepo != nil {
			beforeUser, err := reportRepo.FindUser(r.Context(), login)
			if err == nil {
				beforeCount = len(beforeUser.Notifications)
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		response, err := client.AnalyzeUser(ctx, &proto.AnalyzeRequest{Login: login})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if reportRepo != nil {
			afterUser, err := reportRepo.FindUser(r.Context(), login)
			if err == nil {
				afterCount := len(afterUser.Notifications)
				if afterCount > beforeCount {
					newNotifications := afterUser.Notifications[beforeCount:afterCount]
					pushNotifications(login, newNotifications)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if reportRepo == nil {
			http.Error(w, "report repository unavailable", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		results, err := reportRepo.ListAll(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	})

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("HTTP gateway listening on :%s and forwarding analyze calls to %s", port, grpcAddr)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("gateway stopped: %v", err)
	}
}

func pushNotifications(login string, notifications []appanalysis.Notification) {
	clientsMu.Lock()
	client, exists := clients[login]
	clientsMu.Unlock()
	if !exists {
		return
	}

	for _, notification := range notifications {
		payload := map[string]any{
			"id":         notification.ID,
			"type":       notification.Type,
			"message":    notification.Message,
			"created_at": notification.CreatedAt,
			"read":       notification.Read,
			"login":      login,
			"event_type": fmt.Sprintf("notification:%s", notification.Type),
		}

		client.mu.Lock()
		err := client.conn.WriteJSON(payload)
		client.mu.Unlock()
		if err != nil {
			log.Printf("websocket send error for user %s: %v", login, err)
			return
		}
	}
}
