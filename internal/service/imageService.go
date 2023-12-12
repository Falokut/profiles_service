package service

import (
	"context"
	"net/url"
	"runtime"

	image_processing_service "github.com/Falokut/online_cinema_ticket_office/profiles_service/pkg/image_processing_service/v1/protos"
	image_storage_service "github.com/Falokut/online_cinema_ticket_office/profiles_service/pkg/images_storage_service/v1/protos"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ImagesService interface {
	GetProfilePictureUrl(ctx context.Context, pictureID string) string
	ResizeImage(ctx context.Context, image []byte) ([]byte, error)
	UploadImage(ctx context.Context, image []byte) (string, error)
	DeleteImage(ctx context.Context, pictureID string) error
	ReplaceImage(ctx context.Context, image []byte, pictureID string, createIfNotExist bool) (string, error)
}

type ImagesServiceConfig struct {
	ImageWidth        int32
	ImageHeight       int32
	ImageResizeMethod image_processing_service.ResampleFilter

	BaseProfilePictureUrl  string
	ProfilePictureCategory string

	AllowedTypes   []string
	MaxImageWidth  int32
	MaxImageHeight int32
	MinImageWidth  int32
	MinImageHeight int32
}

type imagesService struct {
	cfg                    ImagesServiceConfig
	logger                 *logrus.Logger
	imageStorageService    image_storage_service.ImagesStorageServiceV1Client
	imageProcessingService image_processing_service.ImageProcessingServiceV1Client
	errorHandler           errorHandler
}

func NewImagesService(cfg ImagesServiceConfig, logger *logrus.Logger,
	imageStorageService image_storage_service.ImagesStorageServiceV1Client,
	imageProcessingService image_processing_service.ImageProcessingServiceV1Client) *imagesService {
	errorHandler := newErrorHandler(logger)
	return &imagesService{
		cfg:                    cfg,
		logger:                 logger,
		imageStorageService:    imageStorageService,
		errorHandler:           errorHandler,
		imageProcessingService: imageProcessingService,
	}
}

// Returns profile picture url for GET request, or
// returns empty string if there are error or picture unreachable
func (s *imagesService) GetProfilePictureUrl(ctx context.Context, pictureID string) string {
	if pictureID == "" {
		return ""
	}

	u, err := url.Parse(s.cfg.BaseProfilePictureUrl)
	if err != nil {
		s.logger.Errorf("can't parse url. error: %s", err.Error())
		return ""
	}

	res, err := s.imageStorageService.IsImageExist(ctx,
		&image_storage_service.ImageRequest{
			Category: s.cfg.ProfilePictureCategory,
			ImageId:  pictureID})
	if err != nil {
		s.logger.Error(err)
		return ""
	}
	if !res.ImageExist {
		return ""
	}

	q := u.Query()
	q.Add("image_id", pictureID)
	q.Add("category", s.cfg.ProfilePictureCategory)
	u.RawQuery = q.Encode()
	return u.String()
}

// returns error if image not valid
func (s *imagesService) checkImage(ctx context.Context, image []byte) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"ImagesService.checkImage")
	defer span.Finish()

	img := &image_processing_service.Image{Image: image}
	res, err := s.imageProcessingService.Validate(ctx, &image_processing_service.ValidateRequest{
		Image:          img,
		SupportedTypes: s.cfg.AllowedTypes,
		MaxWidth:       &s.cfg.MaxImageWidth,
		MaxHeight:      &s.cfg.MaxImageHeight,
		MinHeight:      &s.cfg.MinImageHeight,
		MinWidth:       &s.cfg.MinImageWidth,
	})
	if status.Code(err) == codes.Internal {
		return s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error())
	} else if res != nil && !res.ImageValid || status.Code(err) == codes.InvalidArgument {
		return s.errorHandler.createExtendedErrorResponceWithSpan(span, ErrInvalidImage, "", res.GetDetails())
	}

	span.SetTag("grpc.status", codes.OK)
	return nil
}
func (s *imagesService) ResizeImage(ctx context.Context, image []byte) ([]byte, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"imagesService.ResizeImage")
	defer span.Finish()

	resized, err := s.imageProcessingService.Resize(ctx, &image_processing_service.ResizeRequest{
		Image:          &image_processing_service.Image{Image: image},
		ResampleFilter: s.cfg.ImageResizeMethod,
		Width:          s.cfg.ImageWidth,
		Height:         s.cfg.ImageHeight,
	})

	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return []byte{}, err
	}
	if resized == nil {
		return []byte{}, s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, "can't resize image")
	}

	span.SetTag("grpc.status", codes.OK)
	return resized.Data, nil
}

func (s *imagesService) UploadImage(ctx context.Context, image []byte) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"imagesService.UploadImage")
	defer span.Finish()

	if err := s.checkImage(ctx, image); err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return "", err
	}

	imageSizeWithoutResize := len(image)
	s.logger.Info("Resizing image")
	image, err := s.ResizeImage(ctx, image)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return "", err
	}

	s.logger.Debugf("image size before resizing: %d resized: %d", imageSizeWithoutResize, len(image))

	s.logger.Info("Creating stream")
	stream, err := s.imageStorageService.StreamingUploadImage(ctx)
	if err != nil {
		return "", s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error())
	}

	chunkSize := (len(image) + runtime.NumCPU() - 1) / runtime.NumCPU()
	for i := 0; i < len(image); i += chunkSize {
		last := i + chunkSize
		if last > len(image) {
			last = len(image)
		}
		var chunk []byte
		chunk = append(chunk, image[i:last]...)

		s.logger.Info("Send image chunk")
		err = stream.Send(&image_storage_service.StreamingUploadImageRequest{
			Category: s.cfg.ProfilePictureCategory,
			Data:     chunk,
		})
		if err != nil {
			return "", s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error()+"error while sending streaming message")
		}
	}

	s.logger.Info("Closing stream")
	res, err := stream.CloseAndRecv()
	if err != nil {
		return "", s.errorHandler.createErrorResponceWithSpan(span, ErrInternal, err.Error()+"error while sending close")
	}

	span.SetTag("grpc.status", codes.OK)
	return res.ImageId, nil
}

func (s *imagesService) DeleteImage(ctx context.Context, pictureID string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"imagesService.DeleteImage")
	defer span.Finish()

	s.logger.Debugf("Deleting image with %s id", pictureID)
	_, err := s.imageStorageService.DeleteImage(ctx, &image_storage_service.ImageRequest{
		Category: s.cfg.ProfilePictureCategory,
		ImageId:  pictureID,
	})
	if err != nil {
		return s.errorHandler.createErrorResponceWithSpan(span, err, "")
	}

	span.SetTag("grpc.status", codes.OK)
	return nil
}

func (s *imagesService) ReplaceImage(ctx context.Context, image []byte,
	pictureID string, createIfNotExist bool) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"imagesService.ReplaceImage")
	defer span.Finish()

	if err := s.checkImage(ctx, image); err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return "", err
	}

	uncompressedSize := len(image)
	s.logger.Info("Resizing image")
	image, err := s.ResizeImage(ctx, image)
	if err != nil {
		span.SetTag("grpc.status", status.Code(err))
		ext.LogError(span, err)
		return "", err
	}
	s.logger.Debugf("image size before resizing: %d resized: %d", uncompressedSize, len(image))

	resp, err := s.imageStorageService.ReplaceImage(ctx,
		&image_storage_service.ReplaceImageRequest{
			Category:         s.cfg.ProfilePictureCategory,
			ImageId:          pictureID,
			ImageData:        image,
			CreateIfNotExist: createIfNotExist,
		})

	if err != nil {
		return "", s.errorHandler.createErrorResponceWithSpan(span, err, "")
	}

	span.SetTag("grpc.status", codes.OK)
	return resp.ImageId, nil
}
