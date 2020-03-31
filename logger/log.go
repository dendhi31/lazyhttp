package logger

import "log"

type Logger interface {
	Debugln(args ...interface{})
}

type Config struct {
	Debug bool
}

func New(config Config) Logger {
	return &Config{
		Debug: config.Debug,
	}
}

func (logs *Config) Debugln(args ...interface{}) {
	if logs.Debug {
		log.Println(args...)
	}
}
