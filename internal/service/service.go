package service

import (
	"context"

	"github.com/Falokut/profiles_service/internal/models"
	"github.com/Falokut/profiles_service/internal/repository"
	"github.com/sirupsen/logrus"
)

type ImagesService interface {
	GetProfilePictureURL(ctx context.Context, pictureID string) string
	ResizeImage(ctx context.Context, image []byte) ([]byte, error)
	UploadImage(ctx context.Context, image []byte) (string, error)
	DeleteImage(ctx context.Context, pictureID string) error
	ReplaceImage(ctx context.Context, image []byte, pictureID string, createIfNotExist bool) (string, error)
}

type ProfilesService interface {
	GetProfile(ctx context.Context, accountID string) (models.Profile, error)
	UpdateProfilePicture(ctx context.Context, accountID string, image []byte) error
	GetEmail(ctx context.Context, accountID string) (string, error)
	DeleteProfilePicture(ctx context.Context, accountID string) error
}

type profilesService struct {
	repo          repository.ProfileRepository
	logger        *logrus.Logger
	imagesService ImagesService
}

func NewProfilesService(repo repository.ProfileRepository,
	logger *logrus.Logger, imagesService ImagesService) *profilesService {
	return &profilesService{repo: repo,
		logger:        logger,
		imagesService: imagesService,
	}
}

func (s *profilesService) GetProfile(ctx context.Context,
	accountID string) (profile models.Profile, err error) {
	repoProfile, err := s.repo.GetProfile(ctx, accountID)

	if err != nil {
		return
	}

	profile = models.Profile{
		AccountID:         repoProfile.AccountID,
		Email:             repoProfile.Email,
		Username:          repoProfile.Username,
		RegistrationDate:  repoProfile.RegistrationDate,
		ProfilePictureURL: s.imagesService.GetProfilePictureURL(ctx, repoProfile.ProfilePictureID),
	}
	return
}

func (s *profilesService) UpdateProfilePicture(ctx context.Context,
	accountID string, image []byte) (err error) {
	s.logger.Info("Getting current picture id")
	currentPictureID, err := s.repo.GetProfilePictureID(ctx, accountID)
	if err != nil {
		return
	}

	var pictureID string
	if currentPictureID == "" {
		s.logger.Info("Uploading image")
		pictureID, err = s.imagesService.UploadImage(ctx, image)
	} else {
		s.logger.Info("Replacing image")
		pictureID, err = s.imagesService.ReplaceImage(ctx, image, currentPictureID, true)
	}

	if err != nil {
		return
	}

	if pictureID != currentPictureID {
		s.logger.Info("Updating PictureID")
		err = s.repo.UpdateProfilePictureID(ctx, accountID, pictureID)
		if err != nil {
			return
		}
	}

	return
}

func (s *profilesService) GetEmail(ctx context.Context, accountID string) (email string, err error) {
	email, err = s.repo.GetEmail(ctx, accountID)
	return
}

func (s *profilesService) DeleteProfilePicture(ctx context.Context, accountID string) (err error) {
	currentPictureID, err := s.repo.GetProfilePictureID(ctx, accountID)
	if err != nil || currentPictureID == "" {
		return
	}

	err = s.imagesService.DeleteImage(ctx, currentPictureID)
	if err != nil {
		return
	}

	err = s.repo.UpdateProfilePictureID(ctx, accountID, "")
	return
}
