package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	server "github.com/Falokut/grpc_rest_server"
	"github.com/Falokut/healthcheck"
	"github.com/Falokut/profiles_service/internal/config"
	"github.com/Falokut/profiles_service/internal/events"
	"github.com/Falokut/profiles_service/internal/handler"
	"github.com/Falokut/profiles_service/internal/imagesservice"
	"github.com/Falokut/profiles_service/internal/repository/postgresrepository"
	"github.com/Falokut/profiles_service/internal/service"
	jaegerTracer "github.com/Falokut/profiles_service/pkg/jaeger"
	"github.com/Falokut/profiles_service/pkg/logging"
	"github.com/Falokut/profiles_service/pkg/metrics"
	profiles_service "github.com/Falokut/profiles_service/pkg/profiles_service/v1/protos"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
)

func main() {
	logging.NewEntry(logging.ConsoleOutput)
	logger := logging.GetLogger()
	cfg := config.GetConfig()

	logLevel, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		logger.Fatal(err)
	}
	logger.Logger.SetLevel(logLevel)

	tracer, closer, err := jaegerTracer.InitJaeger(cfg.JaegerConfig)
	if err != nil {
		logger.Errorf("Shutting down, error while creating tracer %v", err)
		return
	}
	logger.Info("Jaeger connected")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	logger.Info("Metrics initializing")
	metric, err := metrics.CreateMetrics(cfg.PrometheusConfig.Name)
	if err != nil {
		logger.Errorf("Shutting down, error while creating metrics %v", err)
		return
	}

	shutdown := make(chan error, 1)
	go func() {
		logger.Info("Metrics server running")
		if err := metrics.RunMetricServer(cfg.PrometheusConfig.ServerConfig); err != nil {
			logger.Errorf("Shutting down, error while running metrics server %v", err)
			shutdown <- err
			return
		}
	}()

	logger.Info("Database initializing")
	database, err := postgresrepository.NewPostgreDB(cfg.DBConfig)
	if err != nil {
		logger.Errorf("Shutting down, connection to the database is not established: %s", err.Error())
		return
	}
	defer database.Close()

	logger.Info("Repository initializing")
	repo := postgresrepository.NewProfilesRepository(database, logger.Logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		logger.Info("Running movie event consumer")
		movieEventsConsumer := events.NewAccountEventsConsumer(getKafkaReaderConfig(cfg.AccountEventsConfig),
			logger.Logger, repo)
		movieEventsConsumer.Run(ctx)
		wg.Done()
	}()

	go func() {
		logger.Info("Healthcheck initializing")
		healthcheckManager := healthcheck.NewHealthManager(logger.Logger,
			[]healthcheck.HealthcheckResource{database}, cfg.HealthcheckPort, nil)
		if err := healthcheckManager.RunHealthcheckEndpoint(); err != nil {
			logger.Errorf("Shutting down, error while running healthcheck endpoint %s", err.Error())
			shutdown <- err
			return
		}
	}()

	imagesService, err := imagesservice.NewImagesService(getImageServiceConfig(cfg),
		logger.Logger, cfg.ImageStorageService.Addr,
		cfg.ImageStorageService.SecureConfig,
		cfg.ImageProcessingService.Addr,
		cfg.ImageProcessingService.SecureConfig)
	if err != nil {
		logger.Errorf("Shutting down, error while creating images service %s", err.Error())
		return
	}
	defer imagesService.Shutdown()

	logger.Info("Service initializing")
	service := service.NewProfilesService(repo, logger.Logger, imagesService)

	handler := handler.NewProfilesServiceHandler(service)

	logger.Info("Server initializing")
	s := server.NewServer(logger.Logger, handler)
	go func() {
		if err := s.Run(getListenServerConfig(cfg), metric, nil, nil); err != nil {
			logger.Errorf("Shutting down, error while running server %s", err.Error())
			shutdown <- err
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGTERM)

	select {
	case <-quit:
		break
	case <-shutdown:
		break
	}

	s.Shutdown()
	cancel()
	wg.Wait()
}

func getImageServiceConfig(cfg *config.Config) imagesservice.ImagesServiceConfig {
	return imagesservice.ImagesServiceConfig{
		ImageWidth:                   cfg.ImageProcessingService.ProfilePictureWidth,
		ImageHeight:                  cfg.ImageProcessingService.ProfilePictureHeight,
		BaseProfilePictureUrl:        cfg.ImageStorageService.BaseProfilePictureUrl,
		ProfilePictureCategory:       cfg.ImageStorageService.ProfilePictureCategory,
		AllowedTypes:                 cfg.ImageProcessingService.AllowedTypes,
		MaxImageWidth:                cfg.ImageProcessingService.MaxImageWidth,
		MaxImageHeight:               cfg.ImageProcessingService.MaxImageHeight,
		MinImageWidth:                cfg.ImageProcessingService.MinImageWidth,
		MinImageHeight:               cfg.ImageProcessingService.MinImageHeight,
		ImageResizeMethod:            cfg.ImageProcessingService.ImageResizeMethod,
		CheckProfilePictureExistance: cfg.ImageStorageService.CheckProfilePictureExistance,
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

func getKafkaReaderConfig(cfg config.KafkaReaderConfig) events.KafkaReaderConfig {
	return events.KafkaReaderConfig{
		Brokers:          cfg.Brokers,
		GroupID:          cfg.GroupID,
		ReadBatchTimeout: cfg.ReadBatchTimeout,
	}
}
