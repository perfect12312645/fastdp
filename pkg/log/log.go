package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Logger *zap.Logger

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func getLogWriter() zapcore.WriteSyncer {
	return zapcore.AddSync(os.Stdout)
}

func InitLogger(debug bool) *zap.Logger {
	consoleWs := getLogWriter()
	encoder := getEncoder()
	var level zapcore.Level
	if debug {
		level = zapcore.DebugLevel
	} else {
		level = zapcore.InfoLevel
	}

	// 创建核心组件
	core := zapcore.NewCore(encoder, consoleWs, level)
	logger := zap.New(core)
	logger.Debug("输出日志位置：终端")
	defer logger.Sync()
	return logger
}
