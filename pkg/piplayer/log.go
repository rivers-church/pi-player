package piplayer

import (
	"io"
	"log/slog"
	"os"
)

// logLevel is the level the player logs at. It lives in a LevelVar so the
// settings page can turn debug logging on and off without a restart.
var logLevel = new(slog.LevelVar)

// logger is where the package logs. Debug lines go through logger.Debug and
// are dropped unless debug logging is on, which replaces the `if debug` guard
// that used to sit in front of every one of them.
var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

// SetDebugLogging turns debug logging on or off.
func SetDebugLogging(on bool) {
	if on {
		logLevel.Set(slog.LevelDebug)
		return
	}
	logLevel.Set(slog.LevelInfo)
}

// discardLogs silences the package logger, for tests that would otherwise
// bury a real failure in websocket chatter.
func discardLogs() {
	logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: logLevel}))
}
