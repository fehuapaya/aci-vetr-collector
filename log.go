package main

import (
	"fmt"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
)

type Logger = zerolog.Logger

func newLogger() Logger {
	file, err := os.Create(logFile)
	if err != nil {
		panic(fmt.Sprintf("cannot create log file %s", logFile))
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	zerolog.DurationFieldInteger = true

	return zerolog.New(zerolog.MultiLevelWriter(
		file,
		zerolog.ConsoleWriter{Out: colorable.NewColorableStdout()},
	)).With().Timestamp().Logger()
}
