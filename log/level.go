package log

import (
	"errors"
)

// LogLevelMapping is a mapping for LogLevel enum
var LogLevelMapping = map[string]LogLevel{
	ERROR.String():   ERROR,
	WARNING.String(): WARNING,
	INFO.String():    INFO,
	DEBUG.String():   DEBUG,
	SILENT.String():  SILENT,
}

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	SILENT
)

type LogLevel int

// UnmarshalYAML unserialize LogLevel with yaml
func (l *LogLevel) UnmarshalYAML(unmarshal func(any) error) error {
	var tp string
	unmarshal(&tp)
	level, exist := LogLevelMapping[tp]
	if !exist {
		return errors.New("invalid mode")
	}
	*l = level
	return nil
}

// MarshalYAML serialize LogLevel with yaml
func (l LogLevel) MarshalYAML() (any, error) {
	return l.String(), nil
}

func (l LogLevel) String() string {
	switch l {
	case INFO:
		return "info"
	case WARNING:
		return "warning"
	case ERROR:
		return "error"
	case DEBUG:
		return "debug"
	case SILENT:
		return "silent"
	default:
		return "unknown"
	}
}
