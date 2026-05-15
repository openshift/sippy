package partman

import (
	"fmt"
	"log/slog"
	"os"
)

type Logger interface {
	Info(args ...interface{})
	Debug(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})

	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// SlogLogger implements the Logger interface using the slog package
type SlogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger creates a new SlogLogger instance
func NewSlogLogger(opts ...slog.HandlerOptions) *SlogLogger {
	var handler slog.Handler

	if len(opts) > 0 {
		handler = slog.NewJSONHandler(os.Stdout, &opts[0])
	} else {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}

	return &SlogLogger{logger: slog.New(handler)}
}

// Info logs an info message
func (l *SlogLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args[0]), args[1:]...)
}

// Debug logs a debug message
func (l *SlogLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args[0]), args[1:]...)
}

// Warn logs a warning message
func (l *SlogLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args[0]), args[1:]...)
}

// Error logs an error message
func (l *SlogLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args[0]), args[1:]...)
}

// Fatal logs a fatal message and exits
func (l *SlogLogger) Fatal(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args[0]), args[1:]...)
	os.Exit(1)
}

// Infof logs a formatted info message
func (l *SlogLogger) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// Debugf logs a formatted debug message
func (l *SlogLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message
func (l *SlogLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message
func (l *SlogLogger) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// Fatalf logs a formatted fatal message and exits
func (l *SlogLogger) Fatalf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
