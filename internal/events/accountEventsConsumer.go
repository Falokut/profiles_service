package events

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Falokut/profiles_service/internal/models"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type accountsEventsConsumer struct {
	reader        *kafka.Reader
	logger        *logrus.Logger
	repo          ProfilesRepository
	imagesService ImagesService
}

const (
	accountCreatedTopic = "account_created"
	accountDeletedTopic = "account_deleted"
)

func NewAccountEventsConsumer(
	cfg KafkaReaderConfig,
	logger *logrus.Logger,
	repo ProfilesRepository,
	imagesService ImagesService) *accountsEventsConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          cfg.Brokers,
		GroupTopics:      []string{accountCreatedTopic, accountDeletedTopic},
		GroupID:          cfg.GroupID,
		Logger:           logger,
		ReadBatchTimeout: cfg.ReadBatchTimeout,
	})

	return &accountsEventsConsumer{
		reader:        r,
		logger:        logger,
		repo:          repo,
		imagesService: imagesService,
	}
}

func (e *accountsEventsConsumer) Run(ctx context.Context) {
	for {
		select {
		default:
			e.Consume(ctx)
		case <-ctx.Done():
			e.logger.Info("account events consumer shutting down")
			e.reader.Close()
			e.logger.Info("account events consumer shutted down")
			return
		}
	}
}

func (e *accountsEventsConsumer) Shutdown() {
	e.logger.Info("Shutting down accounts events counsumer")
	err := e.reader.Close()
	if err != nil {
		e.logger.Errorf("Error occurred while shutting down accounts events counsumer")
	}
}

func (e *accountsEventsConsumer) AccountCreated(ctx context.Context, account models.AccountCreatedDTO) (err error) {
	defer e.handleError(ctx, &err)
	defer e.logError(err, "AccountCreated")

	err = e.repo.CreateProfile(ctx, &models.Profile{
		AccountID:        account.ID,
		Email:            account.Email,
		RegistrationDate: account.RegistrationDate,
		Username:         account.Username,
	})

	return
}

func (e *accountsEventsConsumer) AccountDeleted(ctx context.Context, _, accountID string) (err error) {
	defer e.handleError(ctx, &err)
	defer e.logError(err, "AccountDeleted")

	pictureID, err := e.repo.GetProfilePictureID(ctx, accountID)
	if models.Code(err) == models.NotFound {
		// account already deleted
		return nil
	}

	if err != nil {
		return
	}

	tx, err := e.repo.DeleteProfile(ctx, accountID)
	if err != nil {
		return
	}

	err = e.imagesService.DeleteImage(ctx, pictureID)
	if err != nil {
		err = tx.Rollback()
		return
	}

	err = tx.Commit()
	return
}

func (e *accountsEventsConsumer) handleError(ctx context.Context, err *error) {
	if ctx.Err() != nil {
		var code models.ErrorCode
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			code = models.Canceled
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			code = models.DeadlineExceeded
		}
		*err = models.Error(code, ctx.Err().Error())
		return
	}

	if err == nil || *err == nil {
		return
	}

	var serviceErr = &models.ServiceError{}
	if !errors.As(*err, &serviceErr) {
		*err = models.Error(models.Internal, "error while sending event notification")
	}
}

func (e *accountsEventsConsumer) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var eventsErr = &models.ServiceError{}
	if errors.As(err, &eventsErr) {
		e.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           eventsErr.Msg,
				"error.code":          eventsErr.Code,
			},
		).Error("account events error occurred")
	} else {
		e.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error(),
			},
		).Error("account events error occurred")
	}
}

func (e *accountsEventsConsumer) Consume(ctx context.Context) {
	var err error
	defer e.handleError(ctx, &err)

	message, err := e.reader.FetchMessage(ctx)
	if err != nil {
		return
	}

	switch message.Topic {
	case accountCreatedTopic:
		var account models.AccountCreatedDTO
		err = json.Unmarshal(message.Value, &account)
		if err != nil {
			// skip messages with invalid structure
			err = e.reader.CommitMessages(ctx, message)
			return
		}

		err = e.AccountCreated(ctx, account)
	case accountDeletedTopic:
		var deletedAccount struct {
			Email     string `json:"email"`
			AccountID string `json:"account_id"`
		}

		err = json.Unmarshal(message.Value, &deletedAccount)
		if err != nil {
			// skip messages with invalid structure
			err = e.reader.CommitMessages(ctx, message)
			return
		}
		err = e.AccountDeleted(ctx, deletedAccount.Email, deletedAccount.AccountID)
	}

	if err != nil {
		return
	}

	err = e.reader.CommitMessages(ctx, message)
}
