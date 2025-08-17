package main

import (
	"flag"
	"log"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/crawler/cli/app"
	"github.com/NHYCRaymond/go-backend-kit/logging"
	"github.com/spf13/viper"
)

var (
	configFile  = flag.String("config", "config/tui.yaml", "Configuration file path")
	redisAddr   = flag.String("redis", "", "Override Redis address from config")
	redisPrefix = flag.String("prefix", "", "Override Redis prefix from config")
	mongoURI    = flag.String("mongo", "", "Override MongoDB URI from config")
	logLevel    = flag.String("log", "", "Override log level from config")
	refreshRate = flag.Duration("refresh", 0*time.Second, "Override refresh rate from config")
)

func main() {
	flag.Parse()

	// Load configuration using Viper
	viper.SetConfigFile(*configFile)
	viper.SetConfigType("yaml")
	
	// Set defaults
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.prefix", "crawler")
	viper.SetDefault("database.mongo.uri", "")
	viper.SetDefault("display.refresh_rate", "5s")
	viper.SetDefault("logger.level", "error")
	viper.SetDefault("logger.format", "json")
	viper.SetDefault("logger.output", "none")
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		// Config file is optional for TUI, continue with defaults
		log.Printf("Config file not found or invalid, using defaults: %v", err)
	}
	
	// Apply command line overrides
	redisAddrValue := viper.GetString("redis.address")
	if *redisAddr != "" {
		redisAddrValue = *redisAddr
	}
	
	redisPrefixValue := viper.GetString("redis.prefix")
	if *redisPrefix != "" {
		redisPrefixValue = *redisPrefix
	}
	
	mongoURIValue := viper.GetString("database.mongo.uri")
	if *mongoURI != "" {
		mongoURIValue = *mongoURI
	}
	
	logLevelValue := viper.GetString("logger.level")
	if *logLevel != "" {
		logLevelValue = *logLevel
	}
	
	refreshRateValue := viper.GetDuration("display.refresh_rate")
	if *refreshRate > 0 {
		refreshRateValue = *refreshRate
	}

	// Initialize logging - disable output to avoid TUI corruption
	// For debugging, use a separate debug build or external monitoring
	logConfig := config.LoggerConfig{
		Level:  logLevelValue,
		Format: viper.GetString("logger.format"),
		Output: viper.GetString("logger.output"),
	}
	if err := logging.InitLogger(logConfig); err != nil {
		log.Fatal("Failed to init logger:", err)
	}

	logger := logging.GetLogger()

	// Create TUI application
	appConfig := &app.Config{
		RedisAddr:   redisAddrValue,
		RedisPrefix: redisPrefixValue,
		MongoURI:    mongoURIValue,
		RefreshRate: refreshRateValue,
		Logger:      logger,
	}

	application := app.NewApp(appConfig)

	// Run the application
	if err := application.Run(); err != nil {
		log.Fatal("Failed to run TUI:", err)
	}
}