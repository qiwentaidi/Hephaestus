package crack

import (
	"io"
	"log"
	"os"
)

var packageLogger = log.New(io.Discard, "[crack] ", log.LstdFlags)

// SetLogger replaces the package logger. Nil resets to discard.
func SetLogger(logger *log.Logger) {
	if logger == nil {
		packageLogger = log.New(io.Discard, "[crack] ", log.LstdFlags)
		return
	}
	packageLogger = logger
}

// EnableStdLogger sends package logs to stderr using the standard logger format.
func EnableStdLogger() {
	packageLogger = log.New(os.Stderr, "[crack] ", log.LstdFlags)
}

func infof(format string, args ...interface{}) {
	packageLogger.Printf("INFO "+format, args...)
}

func warningf(format string, args ...interface{}) {
	packageLogger.Printf("WARN "+format, args...)
}

func errorf(format string, args ...interface{}) {
	packageLogger.Printf("ERROR "+format, args...)
}
