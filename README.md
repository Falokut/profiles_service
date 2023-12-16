# Content
+ [Configuration](#configuration)
    + [Params info](#configuration-params-info)
        + [Database config](#database-config)
        + [Jaeger config](#jaeger-config)
        + [Prometheus config](#prometheus-config)
        + [time.Duration](#timeduration-yml-supported-values)
+ [Metrics](#metrics)
+ [Docs](#docs)
+ [Author](#author)
+ [License](#license)

# Configuration

1. [Configure profiles_db](profiles_db/README.md#Configuration)
2. Create .env in root dir and provide DB_PASSWORD
```env
DB_PASSWORD= "YourPasswordForProfilesService"
```

## Configuration params info
if supported values is empty, then any type values are supported

| yml name | yml section | env name | param type| description | supported values |
|-|-|-|-|-|-|
| log_level   |      | LOG_LEVEL  |   string   |      logging level        | panic, fatal, error, warning, warn, info, debug, trace|
| healthcheck_port   |      | HEALTHCHECK_PORT  |   string   |     port for healthcheck| any valid port that is not occupied by other services. The string should not contain delimiters, only the port number|
| host   |  listen    | HOST  |   string   |  ip address or host to listen   |  |
| port   |  listen    | PORT  |   string   |  port to listen   | The string should not contain delimiters, only the port number|
| server_mode   |  listen    | SERVER_MODE  |   string   | Server listen mode, Rest API, gRPC or both | GRPC, REST, BOTH|
| allowed_headers   |  listen    |  |   []string, array of strings   | list of all allowed custom headers. Need for REST API gateway, list of metadata headers, hat are passed through the gateway into the service | any strings list|
|service_name|  prometheus    | PROMETHEUS_SERVICE_NAME | string |  service name, thats will show in prometheus  ||
|server_config|  prometheus    |   | nested yml configuration  [metrics server config](#prometheus-config) | |
|db_config|||nested yml configuration  [database config](#database-config) || configuration for database connection | |
|jaeger|||nested yml configuration  [jaeger config](#jaeger-config)|configuration for jaeger connection ||
|storage_addr|image_storage_service|IMAGE_STORAGE_ADDRESS|string|ip address(or host) with port of image storage service service| all valid addresses formatted like host:port or ip-address:port|
|base_profile_picture_url|image_storage_service|BASE_PROFILE_PICTURE_URL|string|url for getting a profile picture||
|profile_picture_category|image_storage_service|PROFILE_PICTURE_CATEGORY|string|category on storage for profiles picture||
|addr|image_processing_service|IMAGE_PROCESSING_ADDRESS|string|category on storage for profiles picture||
|resize_type|image_processing_service|RESIZE_TYPE|string|resizing method for profile picture|Box,CatmullRom,Lanczos,Linear,MitchellNetravali,NearestNeighbor|
|profile_picture_height|image_processing_service|PROFILE_PICTURE_HEIGHT|int32|picture height after resize|only positive values of int32|
|profile_picture_width|image_processing_service|PROFILE_PICTURE_WIDTH|int32|picture width after resize|only positive values of int32|
|allowed_types|image_processing_service||[]string, array of strings|allowed images mime types|only images mime types (like image/png)|
|max_image_width|image_processing_service|MAX_IMAGE_WIDTH|int32|max profile picture width|only positive values of int32|
|max_image_height|image_processing_service|MAX_IMAGE_HEIGHT|int32|max profile picture height|only positive values of int32|
|min_image_width|image_processing_service|MIN_IMAGE_WIDTH|int32|min profile picture width|only positive values of int32|
|min_image_height|image_processing_service|MIN_IMAGE_HEIGHT|int32|min profile picture height|only positive values of int32|

### Database config
|yml name| env name|param type| description | supported values |
|-|-|-|-|-|
|host|DB_HOST|string|host or ip address of database| |
|port|DB_PORT|string|port of database| any valid port that is not occupied by other services. The string should not contain delimiters, only the port number|
|username|DB_USERNAME|string|username(role) in database||
|password|DB_PASSWORD|string|password for role in database||
|db_name|DB_NAME|string|database name (database instance)||
|ssl_mode|DB_SSL_MODE|string|enable or disable ssl mode for database connection|disabled or enabled|

### Jaeger config

|yml name| env name|param type| description | supported values |
|-|-|-|-|-|
|address|JAEGER_ADDRESS|string|ip address(or host) with port of jaeger service| all valid addresses formatted like host:port or ip-address:port |
|service_name|JAEGER_SERVICE_NAME|string|service name, thats will show in jaeger in traces||
|log_spans|JAEGER_LOG_SPANS|bool|whether to enable log scans in jaeger for this service or not||

### Prometheus config
|yml name| env name|param type| description | supported values |
|-|-|-|-|-|
|host|METRIC_HOST|string|ip address or host to listen for prometheus service||
|port|METRIC_PORT|string|port to listen for  of prometheus service| any valid port that is not occupied by other services. The string should not contain delimiters, only the port number|

# Metrics
The service uses Prometheus and Jaeger and supports distribution tracing

# Docs
[Swagger docs](swagger/docs/profiles_service_v1.swagger.json)
 

# Author

- [@Falokut](https://github.com/Falokut) - Primary author of the project

# License

This project is licensed under the terms of the [MIT License](https://opensource.org/licenses/MIT).

---