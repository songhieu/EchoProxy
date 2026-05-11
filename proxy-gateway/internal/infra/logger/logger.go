package logger

import (
	"os"

	"github.com/rs/zerolog"
)

func New(service string) zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	return zerolog.New(os.Stdout).With().
		Timestamp().
		Str("service", service).
		Logger()
}
