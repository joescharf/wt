package ops

// Logger provides a logging interface for ops functions.
// Consumers implement this to route messages to their UI/logging system.
type Logger interface {
	Info(format string, args ...interface{})
	Success(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Error(format string, args ...interface{})
	Verbose(format string, args ...interface{})
}

// NopLogger discards all log messages.
type NopLogger struct{}

func (NopLogger) Info(string, ...interface{})    {}
func (NopLogger) Success(string, ...interface{}) {}
func (NopLogger) Warning(string, ...interface{}) {}
func (NopLogger) Error(string, ...interface{})   {}
func (NopLogger) Verbose(string, ...interface{})  {}
