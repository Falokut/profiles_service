package imagesservice

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	image_processing_service "github.com/Falokut/image_processing_service/pkg/image_processing_service/v1/protos"
	image_storage_service "github.com/Falokut/images_storage_service/pkg/images_storage_service/v1/protos"
	"github.com/Falokut/profiles_service/internal/config"
	"github.com/Falokut/profiles_service/internal/models"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ImagesServiceConfig struct {
	ImageWidth        int32
	ImageHeight       int32
	ImageResizeMethod string

	BaseProfilePictureURL  string
	ProfilePictureCategory string

	AllowedTypes   []string
	MaxImageWidth  int32
	MaxImageHeight int32
	MinImageWidth  int32
	MinImageHeight int32

	CheckProfilePictureExistance bool
}

type ImagesService struct {
	cfg                    ImagesServiceConfig
	logger                 *logrus.Logger
	imagesStorageService   image_storage_service.ImagesStorageServiceV1Client
	imageProcessingService image_processing_service.ImageProcessingServiceV1Client

	imagesStorageServiceConn   *grpc.ClientConn
	imageProcessingServiceConn *grpc.ClientConn
}

func NewImagesService(cfg *ImagesServiceConfig,
	logger *logrus.Logger,
	imagesStorageAddr string, imageStorageSecureConfig config.ConnectionSecureConfig,
	imageProcessingAddr string, imageProcessingSecureConfig config.ConnectionSecureConfig) (*ImagesService, error) {
	imagesStorageServiceConn, err := getGrpcConn(imagesStorageAddr, imageStorageSecureConfig)
	if err != nil {
		return nil, err
	}

	imageProcessingServiceConn, err := getGrpcConn(imageProcessingAddr, imageProcessingSecureConfig)
	if err != nil {
		imagesStorageServiceConn.Close()
		return nil, err
	}

	return &ImagesService{
		cfg:                        *cfg,
		logger:                     logger,
		imagesStorageServiceConn:   imagesStorageServiceConn,
		imageProcessingServiceConn: imageProcessingServiceConn,
		imagesStorageService:       image_storage_service.NewImagesStorageServiceV1Client(imagesStorageServiceConn),
		imageProcessingService:     image_processing_service.NewImageProcessingServiceV1Client(imageProcessingServiceConn),
	}, nil
}

func (s *ImagesService) Shutdown() {
	if s.imageProcessingServiceConn != nil {
		err := s.imageProcessingServiceConn.Close()
		if err != nil {
			s.logger.Error("error while closing image processing service connection")
		}
	}

	if s.imagesStorageServiceConn != nil {
		err := s.imagesStorageServiceConn.Close()
		if err != nil {
			s.logger.Error("error while closing image storage service connection")
		}
	}
}

func getGrpcConn(addr string, secureConf config.ConnectionSecureConfig) (*grpc.ClientConn, error) {
	creds, err := secureConf.GetGrpcTransportCredentials()
	if err != nil {
		return nil, err
	}
	return grpc.Dial(addr,
		creds,
		grpc.WithUnaryInterceptor(
			otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithStreamInterceptor(
			otgrpc.OpenTracingStreamClientInterceptor(opentracing.GlobalTracer())),
	)
}

// Returns profile picture url for GET request, or
// returns empty string if there are error or picture unreachable
func (s *ImagesService) GetProfilePictureURL(ctx context.Context, pictureID string) string {
	if pictureID == "" {
		return ""
	}

	if s.cfg.CheckProfilePictureExistance {
		res, err := s.imagesStorageService.IsImageExist(ctx,
			&image_storage_service.ImageRequest{
				Category: s.cfg.ProfilePictureCategory,
				ImageId:  pictureID})
		if err != nil {
			s.handleError(ctx, &err, "GetProfilePictureURL")
			return ""
		}
		if !res.ImageExist {
			return ""
		}
	}

	return fmt.Sprintf("%s/%s/%s", s.cfg.BaseProfilePictureURL, s.cfg.ProfilePictureCategory, pictureID)
}

// returns error if image not valid
func (s *ImagesService) checkImage(ctx context.Context, image []byte) (err error) {
	img := &image_processing_service.Image{Image: image}
	res, err := s.imageProcessingService.Validate(ctx, &image_processing_service.ValidateRequest{
		Image:          img,
		SupportedTypes: s.cfg.AllowedTypes,
		MaxWidth:       &s.cfg.MaxImageWidth,
		MaxHeight:      &s.cfg.MaxImageHeight,
		MinHeight:      &s.cfg.MinImageHeight,
		MinWidth:       &s.cfg.MinImageWidth,
	})
	if err != nil {
		var msg string
		if res != nil {
			err = models.Error(models.InvalidArgument, msg)
		} else {
			s.handleError(ctx, &err, "checkImage")
		}
		return
	}
	return nil
}

func (s *ImagesService) ResizeImage(ctx context.Context, image []byte) (resizedImage []byte, err error) {
	defer s.handleError(ctx, &err, "ResizeImage")

	resized, err := s.imageProcessingService.Resize(ctx, &image_processing_service.ResizeRequest{
		Image:          &image_processing_service.Image{Image: image},
		ResampleFilter: convertResizeMethod(s.cfg.ImageResizeMethod),
		Width:          s.cfg.ImageWidth,
		Height:         s.cfg.ImageHeight,
	})

	if err != nil {
		return
	}
	if resized == nil {
		err = models.Error(models.Internal, "error while resizing image")
		return
	}

	return resized.Data, nil
}

func (s *ImagesService) UploadImage(ctx context.Context, image []byte) (imageID string, err error) {
	defer s.handleError(ctx, &err, "UploadImage")
	if err = s.checkImage(ctx, image); err != nil {
		return
	}

	imageSizeWithoutResize := len(image)
	s.logger.Info("Resizing image")
	image, err = s.ResizeImage(ctx, image)
	if err != nil {
		return
	}

	s.logger.Debugf("image size before resizing: %d resized: %d", imageSizeWithoutResize, len(image))
	s.logger.Info("Creating stream")
	stream, err := s.imagesStorageService.StreamingUploadImage(ctx)
	if err != nil {
		return
	}

	chunkSize := (len(image) + runtime.NumCPU() - 1) / runtime.NumCPU()
	for i := 0; i < len(image); i += chunkSize {
		last := i + chunkSize
		if last > len(image) {
			last = len(image)
		}
		var chunk []byte
		chunk = append(chunk, image[i:last]...)

		s.logger.Debug("Sending image chunk")
		err = stream.Send(&image_storage_service.StreamingUploadImageRequest{
			Category: s.cfg.ProfilePictureCategory,
			Data:     chunk,
		})
		if err != nil {
			return
		}
	}

	s.logger.Info("Closing stream")
	res, err := stream.CloseAndRecv()
	if err != nil {
		return
	}

	return res.ImageId, nil
}

func (s *ImagesService) DeleteImage(ctx context.Context, pictureID string) (err error) {
	defer s.handleError(ctx, &err, "DeleteImage")

	s.logger.Debugf("Deleting image with %s id", pictureID)
	_, err = s.imagesStorageService.DeleteImage(ctx, &image_storage_service.ImageRequest{
		Category: s.cfg.ProfilePictureCategory,
		ImageId:  pictureID,
	})

	return
}

func (s *ImagesService) ReplaceImage(ctx context.Context, image []byte,
	pictureID string, createIfNotExist bool) (newPictureID string, err error) {
	defer s.handleError(ctx, &err, "ReplaceImage")

	if err = s.checkImage(ctx, image); err != nil {
		return
	}

	uncompressedSize := len(image)
	s.logger.Info("Resizing image")
	image, err = s.ResizeImage(ctx, image)
	if err != nil {
		return
	}
	s.logger.Debugf("image size before resizing: %d resized: %d", uncompressedSize, len(image))

	resp, err := s.imagesStorageService.ReplaceImage(ctx,
		&image_storage_service.ReplaceImageRequest{
			Category:         s.cfg.ProfilePictureCategory,
			ImageId:          pictureID,
			ImageData:        image,
			CreateIfNotExist: createIfNotExist,
		})

	if err != nil {
		return
	}

	return resp.ImageId, nil
}

func (s *ImagesService) handleError(ctx context.Context, err *error, functionName string) {
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

	s.logError(*err, functionName)
	e := *err
	switch status.Code(*err) {
	case codes.Canceled:
		*err = models.Error(models.Canceled, e.Error())
	case codes.DeadlineExceeded:
		*err = models.Error(models.DeadlineExceeded, e.Error())
	case codes.Internal:
		*err = models.Error(models.Internal, "images service internal error")
	default:
		*err = models.Error(models.Unknown, e.Error())
	}
}

func (s *ImagesService) logError(err error, functionName string) {
	if err == nil {
		return
	}

	var sericeErr = &models.ServiceError{}
	if errors.As(err, &sericeErr) {
		s.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           sericeErr.Msg,
				"code":                sericeErr.Code,
			},
		).Error("images service error occurred")
	} else {
		s.logger.WithFields(
			logrus.Fields{
				"error.function.name": functionName,
				"error.msg":           err.Error,
			},
		).Error("images service error occurred")
	}
}

func convertResizeMethod(method string) image_processing_service.ResampleFilter {
	switch strings.ToTitle(method) {
	case "Box":
		return image_processing_service.ResampleFilter_Box
	case "CatmullRom":
		return image_processing_service.ResampleFilter_CatmullRom
	case "Lanczos":
		return image_processing_service.ResampleFilter_Lanczos
	case "Linear":
		return image_processing_service.ResampleFilter_Linear
	case "MitchellNetravali":
		return image_processing_service.ResampleFilter_MitchellNetravali
	default:
		return image_processing_service.ResampleFilter_NearestNeighbor
	}
}
