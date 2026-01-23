package logger

import (
	"io"
	"os"
	"strings"

	"github.com/singh-gur/api_cache/internal/config"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

// Init initializes the logger based on configuration
func Init(cfg config.LoggingConfig) error {
	Log = logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	Log.SetLevel(level)

	// Set log format
	if strings.ToLower(cfg.Format) == "json" {
		Log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	} else {
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// Set output
	var output io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stderr":
		output = os.Stderr
	case "file":
		if cfg.FilePath == "" {
			return os.ErrInvalid
		}
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		output = file
	default:
		output = os.Stdout
	}
	Log.SetOutput(output)

	return nil
}

// WithFields creates a new logger entry with fields
func WithFields(fields map[string]interface{}) *logrus.Entry {
	return Log.WithFields(fields)
}

// WithField creates a new logger entry with a single field
func WithField(key string, value interface{}) *logrus.Entry {
	return Log.WithField(key, value)
}
