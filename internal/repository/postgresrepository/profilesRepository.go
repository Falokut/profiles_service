package postgresrepository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Falokut/profiles_service/internal/models"
	"github.com/Falokut/profiles_service/internal/repository"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
)

func NewPostgreDB(cfg *repository.DBConfig) (*sqlx.DB, error) {
	conStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.DBName, cfg.SSLMode)
	db, err := sqlx.Connect("pgx", conStr)

	if err != nil {
		return nil, err
	}

	return db, nil
}

type ProfilesRepository struct {
	db     *sqlx.DB
	logger *logrus.Logger
}

const (
	profilesTableName = "profiles"
)

func NewProfilesRepository(db *sqlx.DB, logger *logrus.Logger) *ProfilesRepository {
	return &ProfilesRepository{db: db, logger: logger}
}

func (r *ProfilesRepository) GetProfile(ctx context.Context, accountID string) (profile models.RepositoryProfile, err error) {
	defer r.handleError(ctx, &err, "GetProfile")

	query := fmt.Sprintf(`SELECT username, email, COALESCE(profile_picture_id,'') AS profile_picture_id,registration_date
	 FROM %s WHERE account_id=$1 LIMIT 1;`,
		profilesTableName)

	err = r.db.GetContext(ctx, &profile, query, accountID)
	return
}

func (r *ProfilesRepository) UpdateProfilePictureID(ctx context.Context, accountID, pictureID string) (err error) {
	defer r.handleError(ctx, &err, "UpdateProfilePictureID")

	query := fmt.Sprintf("UPDATE %s SET profile_picture_id=$1 WHERE account_id=$2;",
		profilesTableName)

	_, err = r.db.ExecContext(ctx, query, pictureID, accountID)
	return err
}

func (r *ProfilesRepository) GetProfilePictureID(ctx context.Context, accountID string) (pictureID string, err error) {
	defer r.handleError(ctx, &err, "GetProfilePictureID")

	query := fmt.Sprintf("SELECT COALESCE(profile_picture_id,'')  AS profile_picture_id FROM %s WHERE account_id=$1 LIMIT 1;",
		profilesTableName)

	err = r.db.GetContext(ctx, &pictureID, query, accountID)

	return
}

func (r *ProfilesRepository) GetEmail(ctx context.Context, accountID string) (email string, err error) {
	defer r.handleError(ctx, &err, "GetEmail")
	query := fmt.Sprintf("SELECT email FROM %s WHERE account_id=$1 LIMIT 1;",
		profilesTableName)

	err = r.db.GetContext(ctx, &email, query, accountID)
	return
}

func (r *ProfilesRepository) CreateProfile(ctx context.Context, profile *models.Profile) (err error) {
	defer r.handleError(ctx, &err, "CreateProfile")

	query := fmt.Sprintf("INSERT INTO %s (account_id, email, username, registration_date) VALUES ($1, $2, $3, $4)",
		profilesTableName)

	_, err = r.db.ExecContext(ctx, query, profile.AccountID, profile.Email, profile.Username, profile.RegistrationDate)
	return
}

func (r *ProfilesRepository) DeleteProfile(ctx context.Context, accountID string) (restx repository.Transaction, err error) {
	defer r.handleError(ctx, &err, "DeleteProfile")

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE account_id=$1", profilesTableName)
	_, err = tx.ExecContext(ctx, query, accountID)
	if err != nil {
		err = tx.Rollback()
		return
	}

	return tx, nil
}

func (r *ProfilesRepository) handleError(ctx context.Context, err *error, functionName string) {
	if ctx.Err() != nil {
		var code models.ErrorCode
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			code = models.Canceled
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			code = models.DeadlineExceeded
		}
		*err = models.Error(code, ctx.Err().Error())
		r.logError(*err, functionName)
		return
	}

	if err == nil || *err == nil {
		return
	}

	r.logError(*err, functionName)
	var repoErr = &models.ServiceError{}
	if !errors.As(*err, &repoErr) {
		var code models.ErrorCode
		switch {
		case errors.Is(*err, sql.ErrNoRows):
			code = models.NotFound
			*err = models.Error(code, "profile not found")
		case *err != nil:
			code = models.Internal
			*err = models.Error(code, "repository internal error")
		}
	}
}

func (r *ProfilesRepository) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var repoErr = &models.ServiceError{}
	if errors.As(err, &repoErr) {
		r.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           repoErr.Msg,
				"error.code":          repoErr.Code,
			},
		).Error("profiles repository error occurred")
	} else {
		r.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error(),
			},
		).Error("profiles repository error occurred")
	}
}
