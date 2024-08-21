package main

import (
	"log/slog"
	"os"
)

const defaultLogLevel = slog.LevelInfo

// Set up logging with slog using JSON format. Take a level string the can be
// one of (debug, info, warn, error), an out file where the log will be printet
// to and the addSource boolean which when true will cause slog to print the
// func, file and sourceline of the log call.
func loggingSetup(lvl string, out *os.File, addSource bool) *slog.Logger {
	programLevel := new(slog.LevelVar)
	handlerOpts := slog.HandlerOptions{
		Level:     programLevel,
		AddSource: addSource,
	}

	logger := slog.New(slog.NewJSONHandler(out, &handlerOpts))
	slog.SetDefault(logger)

	if err := programLevel.UnmarshalText([]byte(lvl)); err != nil {
		logger.Error(err.Error())
		programLevel.Set(defaultLogLevel)
	}

	slog.Debug("loglevel set to: " + programLevel.Level().String())
	return logger
}
