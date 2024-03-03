package events

import (
	"context"
	"time"

	"github.com/Falokut/profiles_service/internal/models"
)

type ProfilesRepository interface {
	DeleteProfile(ctx context.Context, id string) error
	CreateProfile(ctx context.Context, profile models.Profile) error
}

type KafkaReaderConfig struct {
	Brokers          []string
	GroupID          string
	ReadBatchTimeout time.Duration
}
