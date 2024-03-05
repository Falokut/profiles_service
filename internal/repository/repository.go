package repository

import (
	"context"

	"github.com/Falokut/profiles_service/internal/models"
)

type Transaction interface {
	Commit() error
	Rollback() error
}

type ProfileRepository interface {
	CreateProfile(ctx context.Context, profile *models.Profile) error
	DeleteProfile(ctx context.Context, accountID string) (tx Transaction, err error)
	GetProfile(ctx context.Context, accountID string) (models.RepositoryProfile, error)
	GetProfilePictureID(ctx context.Context, accountID string) (string, error)
	UpdateProfilePictureID(ctx context.Context, accountID string, pictureID string) error
	GetEmail(ctx context.Context, accountID string) (string, error)
}

type DBConfig struct {
	Host     string `yaml:"host" env:"DB_HOST"`
	Port     string `yaml:"port" env:"DB_PORT"`
	Username string `yaml:"username" env:"DB_USERNAME"`
	Password string `yaml:"password" env:"DB_PASSWORD"`
	DBName   string `yaml:"db_name" env:"DB_NAME"`
	SSLMode  string `yaml:"ssl_mode" env:"DB_SSL_MODE"`
}
