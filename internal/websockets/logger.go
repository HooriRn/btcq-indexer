package websockets

import (
	"github.com/sirupsen/logrus"
)

// TODO(muninn): This is the last dependency of logrus, migrate to midlog.
func NewLogger() *logrus.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{TimestampFormat: "2006-01-02 15:04:05", FullTimestamp: true})
	return l
}
