package midlog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
)

type LogConfig struct {
	Level         Level `yaml:"level"`
	ConsoleLogger bool  `yaml:"console_logger" split_words:"true"`
	NoColor       bool  `yaml:"no_color" split_words:"true"`
}

type Level zerolog.Level

func (l *Level) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	case "debug":
		*l = Level(zerolog.DebugLevel)
	case "info":
		*l = Level(zerolog.InfoLevel)
	case "warning":
		*l = Level(zerolog.WarnLevel)
	default:
		return errors.New(fmt.Sprintf(
			"Bad logging level: %s .Acceptable valus: debug, info, warning", s))
	}
	return nil
}

func (l Level) MarshalYAML() (interface{}, error) {
	var s string
	switch zerolog.Level(l) {
	case zerolog.DebugLevel:
		s = "debug"
	case zerolog.InfoLevel:
		s = "info"
	case zerolog.WarnLevel:
		s = "warning"
	default:
		return nil, errors.New(fmt.Sprintf("Bad logging level: %d", l))
	}

	return s, nil
}
