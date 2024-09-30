/*
Copyright 2024 Netic A/S.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"io"
	"log/slog"
)

const defaultLogLevel = slog.LevelInfo

// loggingSetup sets up logging with slog.
//
// Parameters:
//   - logFormat: A string specifying the log format ("logfmt" or "json").
//   - logLevel: A string specifying the log level ("debug", "info", "warn",
//     "error").
//   - out: An io.Writer where the log will be printed to (e.g. os.Stderr).
//   - addSource: A boolean which, when true, will cause slog to print the
//     function, file, and source line of the log call.
func loggingSetup(logFormat, logLevel string, out io.Writer, addSource bool) *slog.Logger {
	logLeveller := new(slog.LevelVar)
	handlerOpts := slog.HandlerOptions{
		Level:     logLeveller,
		AddSource: addSource,
	}

	var h slog.Handler
	switch logFormat {
	case "json":
		h = slog.NewJSONHandler(out, &handlerOpts)
	case "logfmt":
		h = slog.NewTextHandler(out, &handlerOpts)
	default:
		// Default to JSON
		h = slog.NewJSONHandler(out, &handlerOpts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)

	if err := logLeveller.UnmarshalText([]byte(logLevel)); err != nil {
		logger.Error(err.Error())
		logLeveller.Set(defaultLogLevel)
	}

	slog.Debug("using loglevel " + logLeveller.Level().String())
	return logger
}
