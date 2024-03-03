package config

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	logging "github.com/Falokut/online_cinema_ticket_office.loggerwrapper"
	"github.com/Falokut/profiles_service/internal/repository"
	"github.com/Falokut/profiles_service/pkg/jaeger"
	"github.com/Falokut/profiles_service/pkg/metrics"
	"github.com/ilyakaznacheev/cleanenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type KafkaReaderConfig struct {
	Brokers          []string      `yaml:"brokers"`
	GroupID          string        `yaml:"group_id"`
	ReadBatchTimeout time.Duration `yaml:"read_batch_timeout"`
}

type Config struct {
	LogLevel        string `yaml:"log_level" env:"LOG_LEVEL"`
	HealthcheckPort string `yaml:"healthcheck_port"`
	Listen          struct {
		Host           string   `yaml:"host" env:"HOST"`
		Port           string   `yaml:"port" env:"PORT"`
		Mode           string   `yaml:"server_mode" env:"SERVER_MODE"` // support GRPC, REST, BOTH
		AllowedHeaders []string `yaml:"allowed_headers"`               // Need for REST API gateway, list of metadata headers
	} `yaml:"listen"`

	PrometheusConfig struct {
		Name         string                      `yaml:"service_name" env:"PROMETHEUS_SERVICE_NAME"`
		ServerConfig metrics.MetricsServerConfig `yaml:"server_config"`
	} `yaml:"prometheus"`

	AccountEventsConfig KafkaReaderConfig `yaml:"account_events"`
	ImageStorageService struct {
		Addr                         string                 `yaml:"addr" env:"IMAGE_STORAGE_ADDRESS"`
		SecureConfig                 ConnectionSecureConfig `yaml:"secure_config"`
		BaseProfilePictureUrl        string                 `yaml:"base_profile_picture_url" env:"BASE_PROFILE_PICTURE_URL"`
		ProfilePictureCategory       string                 `yaml:"profile_picture_category" env:"PROFILE_PICTURE_CATEGORY"`
		CheckProfilePictureExistance bool                   `yaml:"check_profile_picture_existance" env:"CHECK_PROFILE_PICTURE_EXISTANCE"`
	} `yaml:"image_storage_service"`
	ImageProcessingService struct {
		Addr                 string                 `yaml:"addr" env:"IMAGE_PROCESSING_ADDRESS"`
		SecureConfig         ConnectionSecureConfig `yaml:"secure_config"`
		ImageResizeMethod    string                 `yaml:"resize_type" env:"RESIZE_TYPE"`
		ProfilePictureHeight int32                  `yaml:"profile_picture_height" env:"PROFILE_PICTURE_HEIGHT"`
		ProfilePictureWidth  int32                  `yaml:"profile_picture_width" env:"PROFILE_PICTURE_WIDTH"`
		AllowedTypes         []string               `yaml:"allowed_types"`
		MaxImageWidth        int32                  `yaml:"max_image_width" env:"MAX_IMAGE_WIDTH"`
		MaxImageHeight       int32                  `yaml:"max_image_height" env:"MAX_IMAGE_HEIGHT"`
		MinImageWidth        int32                  `yaml:"min_image_width" env:"MIN_IMAGE_WIDTH"`
		MinImageHeight       int32                  `yaml:"min_image_height" env:"MIN_IMAGE_HEIGHT"`
	} `yaml:"image_processing_service"`

	DBConfig     repository.DBConfig `yaml:"db_config"`
	JaegerConfig jaeger.Config       `yaml:"jaeger"`
}

const configsPath string = "configs/"

var instance *Config
var once sync.Once

func GetConfig() *Config {
	once.Do(func() {
		logger := logging.GetLogger()
		instance = &Config{}

		if err := cleanenv.ReadConfig(configsPath+"config.yml", instance); err != nil {
			help, _ := cleanenv.GetDescription(instance, nil)
			logger.Fatal(help, " ", err)
		}
	})
	return instance
}

type DialMethod = string

const (
	Insecure                 DialMethod = "INSECURE"
	NilTlsConfig             DialMethod = "NIL_TLS_CONFIG"
	ClientWithSystemCertPool DialMethod = "CLIENT_WITH_SYSTEM_CERT_POOL"
	Server                   DialMethod = "SERVER"
)

type ConnectionSecureConfig struct {
	Method DialMethod `yaml:"dial_method"`
	// Only for client connection with system pool
	ServerName string `yaml:"server_name"`
	CertName   string `yaml:"cert_name"`
	KeyName    string `yaml:"key_name"`
}

func (c ConnectionSecureConfig) GetGrpcTransportCredentials() (grpc.DialOption, error) {
	if c.Method == Insecure {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	if c.Method == NilTlsConfig {
		return grpc.WithTransportCredentials(credentials.NewTLS(nil)), nil
	}

	if c.Method == ClientWithSystemCertPool {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return grpc.EmptyDialOption{}, err
		}
		return grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certPool, c.ServerName)), nil
	}

	cert, err := tls.LoadX509KeyPair(c.CertName, c.KeyName)
	if err != nil {
		return grpc.EmptyDialOption{}, err
	}
	return grpc.WithTransportCredentials(credentials.NewServerTLSFromCert(&cert)), nil
}
