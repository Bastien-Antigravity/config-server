package interfaces

// Logger defines the required logging methods used across the system.
// This interface is compatible with the new universal-logger implementation.
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warning(format string, args ...any)
	Error(format string, args ...any)
	Critical(format string, args ...any)
}
