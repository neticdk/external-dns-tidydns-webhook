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
	"log/slog"
	"os"
)

const defaultLogLevel = slog.LevelInfo

// Set up logging with slog using JSON format. Take a level string the can be
// one of (debug, info, warn, error), an out file where the log will be printet
// to and the addSource boolean which when true will cause slog to print the
// func, file and sourceline of the log call.
func loggingSetup(logFormat, logLevel string, out *os.File, addSource bool) *slog.Logger {
	programLevel := new(slog.LevelVar)
	handlerOpts := slog.HandlerOptions{
		Level:     programLevel,
		AddSource: addSource,
	}

	var h slog.Handler
	if logFormat == "json" {
		h = slog.NewJSONHandler(out, &handlerOpts)
	} else {
		h = slog.NewTextHandler(out, &handlerOpts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)

	if err := programLevel.UnmarshalText([]byte(logLevel)); err != nil {
		logger.Error(err.Error())
		programLevel.Set(defaultLogLevel)
	}

	slog.Debug("using loglevel " + programLevel.Level().String())
	return logger
}
