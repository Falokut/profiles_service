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

func (c *accountsEventsConsumer) Run(ctx context.Context) {
	for {
		select {
		default:
			c.Consume(ctx)
		case <-ctx.Done():
			c.logger.Info("account events consumer shutting down")
			c.reader.Close()
			c.logger.Info("account events consumer shutted down")
			return
		}
	}
}

func (e *accountsEventsConsumer) Shutdown() error {
	return e.reader.Close()
}

func (e *accountsEventsConsumer) AccountCreated(ctx context.Context, account models.Account) (err error) {
	defer e.handleError(ctx, &err)
	defer e.logError(err, "AccountCreated")

	err = e.repo.CreateProfile(ctx, models.Profile{
		AccountId:        account.Id,
		Email:            account.Email,
		RegistrationDate: account.RegistrationDate,
	})

	return
}

func (e *accountsEventsConsumer) AccountDeleted(ctx context.Context, email, accountId string) (err error) {
	defer e.handleError(ctx, &err)
	defer e.logError(err, "AccountDeleted")

	pictureId, err := e.repo.GetProfilePictureId(ctx, accountId)
	if models.Code(err) == models.NotFound {
		// account already deleted
		return nil
	}

	if err != nil {
		return
	}

	tx, err := e.repo.DeleteProfile(ctx, accountId)
	if err != nil {
		return
	}

	err = e.imagesService.DeleteImage(ctx, pictureId)
	if err != nil {
		tx.Rollback()
		return
	}

	tx.Commit()
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

func (c *accountsEventsConsumer) Consume(ctx context.Context) {
	var err error
	defer c.handleError(ctx, &err)

	message, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return
	}

	switch message.Topic {
	case accountCreatedTopic:
		var account models.Account
		err = json.Unmarshal(message.Value, &account)
		if err != nil {
			// skip messages with invalid structure
			err = c.reader.CommitMessages(ctx, message)
			return
		}

		err = c.AccountCreated(ctx, account)
	case accountDeletedTopic:
		var deletedAccount struct {
			Email     string `json:"email"`
			AccountId string `json:"account_id"`
		}

		err = json.Unmarshal(message.Value, &deletedAccount)
		if err != nil {
			// skip messages with invalid structure
			err = c.reader.CommitMessages(ctx, message)
			return
		}
		err = c.AccountDeleted(ctx, deletedAccount.Email, deletedAccount.AccountId)
	}

	if err != nil {
		return
	}

	err = c.reader.CommitMessages(ctx, message)
}
