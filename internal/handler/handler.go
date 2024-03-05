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
	s service.ProfilesService
	profiles_service.UnimplementedProfilesServiceV1Server
}

func NewProfilesServiceHandler(s service.ProfilesService) *ProfileServiceHandler {
	return &ProfileServiceHandler{s: s}
}

func (h *ProfileServiceHandler) GetProfile(ctx context.Context,
	_ *emptypb.Empty) (res *profiles_service.GetProfileResponse, err error) {
	defer h.handleError(&err)

	accountID, err := getAccountID(ctx)
	if err != nil {
		return
	}

	profile, err := h.s.GetProfile(ctx, accountID)
	if err != nil {
		return
	}

	return &profiles_service.GetProfileResponse{
		Username:          profile.Username,
		Email:             profile.Email,
		ProfilePictureURL: profile.ProfilePictureURL,
		RegistrationDate:  timestamppb.New(profile.RegistrationDate),
	}, nil
}

func (h *ProfileServiceHandler) UpdateProfilePicture(ctx context.Context,
	in *profiles_service.UpdateProfilePictureRequest) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)

	accountID, err := getAccountID(ctx)
	if err != nil {
		return
	}

	err = h.s.UpdateProfilePicture(ctx, accountID, in.Image)
	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

func (h *ProfileServiceHandler) GetEmail(ctx context.Context, _ *emptypb.Empty) (res *profiles_service.GetEmailResponse, err error) {
	defer h.handleError(&err)

	accountID, err := getAccountID(ctx)
	if err != nil {
		return
	}
	email, err := h.s.GetEmail(ctx, accountID)
	if err != nil {
		return
	}

	return &profiles_service.GetEmailResponse{Email: email}, nil
}

func (h *ProfileServiceHandler) DeleteProfilePicture(ctx context.Context, _ *emptypb.Empty) (res *emptypb.Empty, err error) {
	defer h.handleError(&err)

	accountID, err := getAccountID(ctx)
	if err != nil {
		return
	}

	err = h.s.DeleteProfilePicture(ctx, accountID)
	if err != nil {
		return
	}

	return &emptypb.Empty{}, nil
}

const (
	AccountIDContext = "X-Account-Id"
)

func getAccountID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no context metadata provided")
	}

	accountID := md.Get(AccountIDContext)
	if len(accountID) == 0 || accountID[0] == "" {
		return "", status.Error(codes.Unauthenticated, "no account id provided")
	}

	return accountID[0], nil
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
