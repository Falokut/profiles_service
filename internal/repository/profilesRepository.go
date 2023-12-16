package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Falokut/profiles_service/internal/model"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/opentracing/opentracing-go"
)

type profilesRepository struct {
	db *sqlx.DB
}

const (
	profilesTableName = "profiles"
)

func NewProfilesRepository(db *sqlx.DB) *profilesRepository {
	return &profilesRepository{db: db}
}

func (r *profilesRepository) GetUserProfile(ctx context.Context, accountID string) (model.UserProfile, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "profilesRepository.GetUserProfile")
	defer span.Finish()
	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("SELECT username, email, profile_picture_id, registration_date FROM %s WHERE account_id=$1 LIMIT 1;",
		profilesTableName)

	var Profile model.UserProfile
	err = r.db.GetContext(ctx, &Profile, query, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.UserProfile{}, ErrProfileNotFound
	}
	return Profile, err
}

func (r *profilesRepository) UpdateProfilePictureID(ctx context.Context, accountID string, pictureID string) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "profilesRepository.GetUserProfile")
	defer span.Finish()

	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("UPDATE %s SET profile_picture_id=$1 WHERE account_id=$2;",
		profilesTableName)

	_, err = r.db.ExecContext(ctx, query, pictureID, accountID)
	return err
}

func (r *profilesRepository) GetProfilePictureID(ctx context.Context, accountID string) (string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx,
		"profilesRepository.GetProfilePictureID")
	defer span.Finish()
	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("SELECT profile_picture_id FROM %s WHERE account_id=$1 LIMIT 1;",
		profilesTableName)

	var pictureID []sql.NullString
	err = r.db.SelectContext(ctx, &pictureID, query, accountID)
	if err != nil {
		return "", err
	}

	if len(pictureID) == 0 || !pictureID[0].Valid {
		return "", nil
	}

	return pictureID[0].String, nil
}

func (r *profilesRepository) GetEmail(ctx context.Context, accountID string) (string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx,
		"profilesRepository.GetEmail")
	defer span.Finish()

	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("SELECT email FROM %s WHERE account_id=$1 LIMIT 1;",
		profilesTableName)

	var Email []sql.NullString
	err = r.db.SelectContext(ctx, &Email, query, accountID)
	if err != nil {
		return "", err
	}

	if len(Email) == 0 || !Email[0].Valid {
		return "", nil
	}

	return Email[0].String, nil
}

func (r *profilesRepository) CreateUserProfile(ctx context.Context, profile model.UserProfile) error {
	span, _ := opentracing.StartSpanFromContext(ctx,
		"profilesRepository.CreateUserProfile")
	defer span.Finish()

	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("INSERT INTO %s (account_id, email, username, registration_date) VALUES ($1, $2, $3, $4)",
		profilesTableName)

	_, err = r.db.QueryContext(ctx, query, profile.AccountID, profile.Email, profile.Username, profile.RegistrationDate)
	return err
}

func (r *profilesRepository) DeleteUserProfile(ctx context.Context, accountID string) error {
	span, _ := opentracing.StartSpanFromContext(ctx,
		"profilesRepository.DeleteUserProfile")
	defer span.Finish()

	var err error
	defer span.SetTag("error", err != nil)

	query := fmt.Sprintf("DELETE FROM %s WHERE account_id=$1", profilesTableName)
	_, err = r.db.ExecContext(ctx, query, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProfileNotFound
	}
	return err
}

func (r *profilesRepository) Shutdown() error {
	return r.db.Close()
}
