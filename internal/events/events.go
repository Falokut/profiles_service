package events

import (
	"context"
	"time"

	"github.com/Falokut/profiles_service/internal/models"
	"github.com/Falokut/profiles_service/internal/repository"
)

type ProfilesRepository interface {
	DeleteProfile(ctx context.Context, id string) (tx repository.Transaction, err error)
	GetProfilePictureID(ctx context.Context, accountID string) (string, error)
	CreateProfile(ctx context.Context, profile *models.Profile) error
}

type KafkaReaderConfig struct {
	Brokers          []string
	GroupID          string
	ReadBatchTimeout time.Duration
}

type ImagesService interface {
	DeleteImage(ctx context.Context, pictureID string) error
}
