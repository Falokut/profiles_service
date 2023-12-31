package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	server "github.com/Falokut/grpc_rest_server"
	"github.com/Falokut/healthcheck"
	image_processing_service "github.com/Falokut/image_processing_service/pkg/image_processing_service/v1/protos"
	image_storage_service "github.com/Falokut/images_storage_service/pkg/images_storage_service/v1/protos"
	logging "github.com/Falokut/online_cinema_ticket_office.loggerwrapper"
	"github.com/Falokut/profiles_service/internal/config"
	"github.com/Falokut/profiles_service/internal/repository"
	"github.com/Falokut/profiles_service/internal/service"
	jaegerTracer "github.com/Falokut/profiles_service/pkg/jaeger"
	"github.com/Falokut/profiles_service/pkg/metrics"
	profiles_service "github.com/Falokut/profiles_service/pkg/profiles_service/v1/protos"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logging.NewEntry(logging.FileAndConsoleOutput)
	logger := logging.GetLogger()
	appCfg := config.GetConfig()
	log_level, err := logrus.ParseLevel(appCfg.LogLevel)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Logger.SetLevel(log_level)

	tracer, closer, err := jaegerTracer.InitJaeger(appCfg.JaegerConfig)
	if err != nil {
		logger.Fatal("cannot create tracer", err)
	}
	logger.Info("Jaeger connected")
	defer closer.Close()

	opentracing.SetGlobalTracer(tracer)

	logger.Info("Metrics initializing")
	metric, err := metrics.CreateMetrics(appCfg.PrometheusConfig.Name)
	if err != nil {
		logger.Fatal(err)
	}

	go func() {
		logger.Info("Metrics server running")
		if err := metrics.RunMetricServer(appCfg.PrometheusConfig.ServerConfig); err != nil {
			logger.Fatal(err)
		}
	}()

	logger.Info("Database initializing")
	database, err := repository.NewPostgreDB(appCfg.DBConfig)
	if err != nil {
		logger.Fatalf("Shutting down, connection to the database is not established: %s", err.Error())
	}

	logger.Info("Repository initializing")
	repo := repository.NewProfilesRepository(database)
	defer repo.Shutdown()

	logger.Info("GRPC Client initializing")
	imageStorageConn, err := getGrpcConn(appCfg.ImageStorageService.StorageAddr)
	if err != nil {
		logger.Fatal(err)
	}
	defer imageStorageConn.Close()

	imageStorageService := image_storage_service.NewImagesStorageServiceV1Client(imageStorageConn)

	imageProcessingConn, err := getGrpcConn(appCfg.ImageProcessingService.Addr)
	if err != nil {
		logger.Fatal(err)
	}
	defer imageProcessingConn.Close()

	imageProcessingService := image_processing_service.NewImageProcessingServiceV1Client(imageProcessingConn)
	logger.Info("Healthcheck initializing")
	healthcheckManager := healthcheck.NewHealthManager(logger.Logger,
		[]healthcheck.HealthcheckResource{database}, appCfg.HealthcheckPort, nil)
	go func() {
		logger.Info("Healthcheck server running")
		if err := healthcheckManager.RunHealthcheckEndpoint(); err != nil {
			logger.Fatalf("Shutting down, can't run healthcheck endpoint %s", err.Error())
		}
	}()

	imagesService := service.NewImagesService(getImageServiceConfig(appCfg),
		logger.Logger, imageStorageService, imageProcessingService)

	logger.Info("Service initializing")
	service := service.NewProfilesService(repo, logger.Logger, imagesService)

	logger.Info("Server initializing")
	s := server.NewServer(logger.Logger, service)
	s.Run(getListenServerConfig(appCfg), metric, nil, nil)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGTERM)

	<-quit
	s.Shutdown()
}

func getGrpcConn(addr string) (*grpc.ClientConn, error) {
	return grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(
			otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithStreamInterceptor(
			otgrpc.OpenTracingStreamClientInterceptor(opentracing.GlobalTracer())),
	)
}

func getImageServiceConfig(cfg *config.Config) service.ImagesServiceConfig {
	return service.ImagesServiceConfig{
		ImageWidth:             cfg.ImageProcessingService.ProfilePictureWidth,
		ImageHeight:            cfg.ImageProcessingService.ProfilePictureHeight,
		ImageResizeMethod:      ConvertResizeType(cfg.ImageProcessingService.ImageResizeMethod),
		BaseProfilePictureUrl:  cfg.ImageStorageService.BaseProfilePictureUrl,
		ProfilePictureCategory: cfg.ImageStorageService.ProfilePictureCategory,
		AllowedTypes:           cfg.ImageProcessingService.AllowedTypes,
		MaxImageWidth:          cfg.ImageProcessingService.MaxImageWidth,
		MaxImageHeight:         cfg.ImageProcessingService.MaxImageHeight,
		MinImageWidth:          cfg.ImageProcessingService.MinImageWidth,
		MinImageHeight:         cfg.ImageProcessingService.MinImageHeight,
	}
}

func getListenServerConfig(cfg *config.Config) server.Config {
	return server.Config{
		Mode:           cfg.Listen.Mode,
		Host:           cfg.Listen.Host,
		Port:           cfg.Listen.Port,
		AllowedHeaders: cfg.Listen.AllowedHeaders,
		ServiceDesc:    &profiles_service.ProfilesServiceV1_ServiceDesc,
		RegisterRestHandlerServer: func(ctx context.Context, mux *runtime.ServeMux, service any) error {
			serv, ok := service.(profiles_service.ProfilesServiceV1Server)
			if !ok {
				return errors.New("can't convert")
			}
			return profiles_service.RegisterProfilesServiceV1HandlerServer(context.Background(),
				mux, serv)
		},
	}
}

func ConvertResizeType(resizeType string) image_processing_service.ResampleFilter {
	resizeType = strings.ToTitle(resizeType)
	switch resizeType {
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
