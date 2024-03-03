package handler

import (
	"context"
	"errors"

	"github.com/Falokut/profiles_service/internal/models"
	"github.com/Falokut/profiles_service/internal/service"
	profiles_service "github.com/Falokut/profiles_service/pkg/profiles_service/v1/protos"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ProfileServiceHandler struct {
	service service.ProfilesService
	profiles_service.UnimplementedProfilesServiceV1Server
}

func NewProfilesServiceHandler(service service.ProfilesService) *ProfileServiceHandler {
	return &ProfileServiceHandler{service: service}
}

func (h *ProfileServiceHandler) GetProfile(ctx context.Context,
	in *emptypb.Empty) (res *profiles_service.GetProfileResponse, err error) {
	defer h.handleError(&err)

	accountId, err := getAccountId(ctx)
	if err != nil {
		return
	}

	profile, err := h.service.GetProfile(ctx, accountId)
	if err != nil {
		return
	}

	return &profiles_service.GetProfileResponse{
		Username:          profile.Username,
		Email:             profile.Email,
		ProfilePictureURL: profile.ProfilePictureUrl,
		RegistrationDate:  timestamppb.New(profile.RegistrationDate),
	}, nil
}

func (h *ProfileServiceHandler) UpdateProfilePicture(ctx context.Context,
	in *profiles_service.UpdateProfilePictureRequest) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)

	accountId, err := getAccountId(ctx)
	if err != nil {
		return
	}

	err = h.service.UpdateProfilePicture(ctx, accountId, in.Image)
	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

func (h *ProfileServiceHandler) GetEmail(ctx context.Context, in *emptypb.Empty) (res *profiles_service.GetEmailResponse, err error) {
	defer h.handleError(&err)

	accountId, err := getAccountId(ctx)
	if err != nil {
		return
	}
	email, err := h.service.GetEmail(ctx, accountId)
	if err != nil {
		return
	}

	return &profiles_service.GetEmailResponse{Email: email}, nil
}

func (h *ProfileServiceHandler) DeleteProfilePicture(ctx context.Context, in *emptypb.Empty) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)

	accountId, err := getAccountId(ctx)
	if err != nil {
		return
	}

	err = h.service.DeleteProfilePicture(ctx, accountId)
	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

const (
	AccountIdContext = "X-Account-Id"
)

func getAccountId(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no context metadata provided")
	}

	accountId := md.Get(AccountIdContext)
	if len(accountId) == 0 || accountId[0] == "" {
		return "", status.Error(codes.Unauthenticated, "no account id provided")
	}

	return accountId[0], nil
}

func (h *ProfileServiceHandler) handleError(err *error) {
	if err == nil || *err == nil {
		return
	}

	serviceErr := &models.ServiceError{}
	if errors.As(*err, &serviceErr) {
		*err = status.Error(convertServiceErrCodeToGrpc(serviceErr.Code), serviceErr.Msg)
	} else if _, ok := status.FromError(*err); !ok {
		e := *err
		*err = status.Error(codes.Unknown, e.Error())
	}
}

func convertServiceErrCodeToGrpc(code models.ErrorCode) codes.Code {
	switch code {
	case models.Internal:
		return codes.Internal
	case models.InvalidArgument:
		return codes.InvalidArgument
	case models.Unauthenticated:
		return codes.Unauthenticated
	case models.Conflict:
		return codes.AlreadyExists
	case models.NotFound:
		return codes.NotFound
	case models.Canceled:
		return codes.Canceled
	case models.DeadlineExceeded:
		return codes.DeadlineExceeded
	case models.PermissionDenied:
		return codes.PermissionDenied
	default:
		return codes.Unknown
	}
}
