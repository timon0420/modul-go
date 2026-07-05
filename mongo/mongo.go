package mongo

import (
	"context"
	"errors"
	"os"
	"time"

	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	URI        string
	Database   string
	Collection string
}

func Connect(ctx context.Context, cfg Config) (*mongodriver.Client, *mongodriver.Collection, error) {
	if cfg.URI == "" {
		cfg.URI = os.Getenv("MONGO_URI")
	}
	if cfg.URI == "" {
		return nil, nil, errors.New("MONGO_URI is not set")
	}
	if cfg.Database == "" {
		cfg.Database = "digital-activities"
	}
	if cfg.Collection == "" {
		cfg.Collection = "activities"
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongodriver.Connect(connectCtx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, nil, err
	}

	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, err
	}

	return client, client.Database(cfg.Database).Collection(cfg.Collection), nil
}
