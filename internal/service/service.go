package service

import (
	"context"
	"errors"

	"github.com/Falokut/profiles_service/internal/model"
	"github.com/Falokut/profiles_service/internal/repository"
	profiles_service "github.com/Falokut/profiles_service/pkg/profiles_service/v1/protos"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ProfilesService struct {
	profiles_service.UnimplementedProfilesServiceV1Server
	repo          repository.ProfileRepository
	logger        *logrus.Logger
	errorHandler  errorHandler
	imagesService ImagesService
}

func NewProfilesService(repo repository.ProfileRepository,
	logger *logrus.Logger, imagesService ImagesService) *ProfilesService {
	errorHandler := newErrorHandler(logger)
	return &ProfilesService{repo: repo,
		logger:        logger,
		errorHandler:  errorHandler,
		imagesService: imagesService,
	}
}

func (s *ProfilesService) GetUserProfile(ctx context.Context,
	in *emptypb.Empty) (*profiles_service.GetUserProfileResponce, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ProfilesService.GetUserProfile")
	defer span.Finish()

	accountID, err := s.getAccountId(ctx)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	Profile, err := s.repo.GetUserProfile(ctx, accountID)
	if errors.Is(err, repository.ErrProfileNotFound) {
		return nil, s.errorHandler.createExtendedErrorResponceWithSpan(span, ErrProfileNotFound, "", ErrProfileNotFound.Error())
	}
	if err != nil {
		return nil, s.errorHandler.createExtendedErrorResponceWithSpan(span, ErrInternal, err.Error(), "")
	}

	span.SetTag("grpc.status", codes.OK)
	return s.convertUserProfileProtoFromModel(ctx, Profile), nil
}

func (s *ProfilesService) UpdateProfilePicture(ctx context.Context,
	in *profiles_service.UpdateProfilePictureRequest) (*emptypb.Empty, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"ProfilesService.UpdateProfilePicture")
	defer span.Finish()

	s.logger.Info("Getting account id from context")
	accountID, err := s.getAccountId(ctx)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	s.logger.Info("Getting current picture id")
	CurrentPictureID, err := s.getCurrentProfileID(ctx, accountID)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	var PictureID string
	if len(CurrentPictureID) == 0 {
		s.logger.Info("Uploading image")
		PictureID, err = s.imagesService.UploadImage(ctx, in.Image)
	} else {
		s.logger.Info("Replacing image")
		PictureID, err = s.imagesService.ReplaceImage(ctx, in.Image, CurrentPictureID, true)
	}

	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	if PictureID != CurrentPictureID {
		s.logger.Info("Updating PictureID")
		err = s.repo.UpdateProfilePictureID(ctx, accountID, PictureID)
		if err != nil {
			return nil, s.errorHandler.createExtendedErrorResponceWithSpan(span,
				ErrInternal, err.Error(), "error while updating picture")
		}
	}

	span.SetTag("grpc.status", codes.OK)
	return &emptypb.Empty{}, nil
}

func (s *ProfilesService) GetEmail(ctx context.Context, in *emptypb.Empty) (*profiles_service.GetEmailResponce, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ProfilesService.GetEmail")
	defer span.Finish()

	accountID, err := s.getAccountId(ctx)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	email, err := s.repo.GetEmail(ctx, accountID)
	if err != nil {
		return nil, s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error())
	}
	if len(email) < 1 {
		return nil, s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, "")
	}

	span.SetTag("grpc.status", codes.OK)
	return &profiles_service.GetEmailResponce{Email: email}, nil
}

func (s *ProfilesService) CreateProfile(ctx context.Context, in *profiles_service.CreateProfileRequest) (*emptypb.Empty, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ProfilesService.CreateProfile")
	defer span.Finish()

	if err := s.repo.CreateUserProfile(ctx, convertUserProfileModelFromProto(in)); err != nil {
		return nil, s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error())
	}

	span.SetTag("grpc.status", codes.OK)
	return &emptypb.Empty{}, nil
}

func (s *ProfilesService) DeleteProfile(ctx context.Context, in *profiles_service.DeleteProfileRequest) (*emptypb.Empty, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ProfilesService.DeleteProfile")
	defer span.Finish()

	err := s.repo.DeleteUserProfile(ctx, in.AccountID)
	if errors.Is(err, repository.ErrProfileNotFound) {
		return nil, s.errorHandler.createExtendedErrorResponceWithSpan(span, ErrProfileNotFound,
			"", ErrProfileNotFound.Error())
	}
	if err != nil {
		return nil, s.errorHandler.createExtendedErrorResponceWithSpan(span, ErrInternal, err.Error(), "")
	}

	span.SetTag("grpc.status", codes.OK)
	return &emptypb.Empty{}, nil
}

const (
	AccountIdContext = "X-Account-Id"
)

func (s *ProfilesService) getAccountId(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", s.errorHandler.createErrorResponce(ErrNoCtxMetaData, "no context metadata provided")
	}

	accountId := md.Get(AccountIdContext)
	if len(accountId) == 0 || accountId[0] == "" {
		return "", s.errorHandler.createErrorResponce(ErrInvalidAccountId, "no account id provided")
	}

	return accountId[0], nil
}

func (s *ProfilesService) DeleteProfilePicture(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"ProfilesService.DeleteProfilePicture")
	defer span.Finish()

	s.logger.Info("Getting account id from context")
	accountID, err := s.getAccountId(ctx)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	CurrentPictureID, err := s.getCurrentProfileID(ctx, accountID)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	if CurrentPictureID == "" {
		return &emptypb.Empty{}, nil
	}

	err = s.imagesService.DeleteImage(ctx, CurrentPictureID)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return nil, err
	}

	err = s.repo.UpdateProfilePictureID(ctx, accountID, "")
	if err != nil {
		return nil, s.errorHandler.createErrorResponceWithSpan(span, ErrCantUpdateProfilePicture, err.Error())
	}

	span.SetTag("grpc.status", codes.OK)
	return &emptypb.Empty{}, nil
}

func (s *ProfilesService) convertUserProfileProtoFromModel(ctx context.Context, from model.UserProfile) *profiles_service.GetUserProfileResponce {
	var ProfilePictureURL string
	if from.ProfilePictureID.Valid {
		ProfilePictureURL = s.imagesService.GetProfilePictureUrl(ctx, from.ProfilePictureID.String)
	}

	return &profiles_service.GetUserProfileResponce{
		Username:          from.Username,
		Email:             from.Email,
		ProfilePictureURL: ProfilePictureURL,
		RegistrationDate:  timestamppb.New(from.RegistrationDate),
	}
}

func convertUserProfileModelFromProto(from *profiles_service.CreateProfileRequest) model.UserProfile {
	return model.UserProfile{
		AccountID:        from.AccountID,
		Username:         from.Username,
		Email:            from.Email,
		RegistrationDate: from.RegistrationDate.AsTime(),
	}
}

func (s *ProfilesService) getCurrentProfileID(ctx context.Context, accountID string) (string, error) {
	CurrentPictureID, err := s.repo.GetProfilePictureID(ctx, accountID)
	if err != nil {
		return "", s.errorHandler.createExtendedErrorResponce(ErrInternal, err.Error(), "can't get current profile picture")
	}

	return CurrentPictureID, nil
}
