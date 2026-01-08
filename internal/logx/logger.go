package logx

import (
	"os"

	"go.uber.org/zap"
)

var L *zap.Logger

func Init() {
	cfg := zap.NewProductionConfig()

	// Local dev readability
	if os.Getenv("ENV") != "prod" {
		cfg = zap.NewDevelopmentConfig()
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	L = logger
}
