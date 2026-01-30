package midlog

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

func LogCommandLine() {
	fmt.Printf("Command: %s\n", strings.Join(os.Args, " "))
}

func SetLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
}

///////////////////// Global utility functions

func DebugT(t Tag, msg string) {
	GlobalLogger.DebugT(t, msg)
}

func Debug(msg string) {
	GlobalLogger.Debug(msg)
}

func DebugF(format string, v ...interface{}) {
	GlobalLogger.DebugF(format, v...)
}

func InfoT(t Tag, msg string) {
	GlobalLogger.InfoT(t, msg)
}

func InfoTF(t Tag, format string, v ...interface{}) {
	GlobalLogger.InfoTF(t, format, v...)
}

func Info(msg string) {
	GlobalLogger.Info(msg)
}

func InfoF(format string, v ...interface{}) {
	GlobalLogger.InfoF(format, v...)
}

func Warn(msg string) {
	GlobalLogger.Warn(msg)
}

func WarnF(format string, v ...interface{}) {
	GlobalLogger.WarnF(format, v...)
}

func WarnT(t Tag, msg string) {
	GlobalLogger.WarnT(t, msg)
}

func WarnTF(t Tag, format string, v ...interface{}) {
	GlobalLogger.WarnTF(t, format, v...)
}

func Error(msg string) {
	GlobalLogger.Error(msg)
}

func ErrorE(err error, msg string) {
	GlobalLogger.ErrorE(err, msg)
}

func ErrorF(format string, v ...interface{}) {
	GlobalLogger.ErrorF(format, v...)
}

func ErrorEF(err error, format string, v ...interface{}) {
	GlobalLogger.ErrorEF(err, format, v...)
}

func Fatal(msg string) {
	GlobalLogger.Fatal(msg)
}

func FatalE(err error, msg string) {
	GlobalLogger.FatalE(err, msg)
}

func FatalF(format string, v ...interface{}) {
	GlobalLogger.FatalF(format, v...)
}

func FatalEF(err error, format string, v ...interface{}) {
	GlobalLogger.FatalEF(err, format, v...)
}

//////////////////// Tags
// Tags are additional fields added to the logs.
// These are usually called Fields, but F in InfoF already refers to format string,
// therefore we refer to them as Tags and call the function InfoT.

type Tag interface {
	apply(logEvent *zerolog.Event)
}

func Int64(key string, value int64) Tag {
	return tagInt64{key, value}
}

func Int(key string, value int) Tag {
	return tagInt{key, value}
}

func Str(key string, value string) Tag {
	return tagStr{key, value}
}

func Float32(key string, value float32) Tag {
	return tagFloat32{key, value}
}

func Float64(key string, value float64) Tag {
	return tagFloat64{key, value}
}

func Err(err error) Tag {
	return tagErr{err}
}

func Tags(tags ...Tag) Tag {
	return multiTag{tags: tags}
}

//////////////// Logger

type Logger struct {
	zlog zerolog.Logger
}

func (l Logger) GetZeroLogger() zerolog.Logger {
	return l.zlog
}

func (l Logger) Debug(msg string) {
	write(l.zlog.Debug(), msg)
}

func (l Logger) DebugF(format string, v ...interface{}) {
	writeF(l.zlog.Debug(), format, v...)
}

func (l Logger) DebugT(t Tag, msg string) {
	writeT(l.zlog.Debug(), t, msg)
}

func (l Logger) Info(msg string) {
	write(l.zlog.Info(), msg)
}

func (l Logger) InfoF(format string, v ...interface{}) {
	writeF(l.zlog.Info(), format, v...)
}

func (l Logger) InfoT(t Tag, msg string) {
	writeT(l.zlog.Info(), t, msg)
}

func (l Logger) InfoTF(t Tag, format string, v ...interface{}) {
	writeTF(l.zlog.Info(), t, format, v...)
}

func (l Logger) Warn(msg string) {
	write(l.zlog.Warn(), msg)
}

func (l Logger) WarnF(format string, v ...interface{}) {
	writeF(l.zlog.Warn(), format, v...)
}

func (l Logger) WarnT(t Tag, msg string) {
	writeT(l.zlog.Warn(), t, msg)
}

func (l Logger) WarnTF(t Tag, format string, v ...interface{}) {
	writeTF(l.zlog.Warn(), t, format, v...)
}

func (l Logger) Error(msg string) {
	write(l.zlog.Error(), msg)
}

func (l Logger) ErrorE(err error, msg string) {
	writeE(l.zlog.Error(), err, msg)
}

func (l Logger) ErrorF(format string, v ...interface{}) {
	writeF(l.zlog.Error(), format, v...)
}

func (l Logger) ErrorEF(err error, format string, v ...interface{}) {
	writeEF(l.zlog.Error(), err, format, v...)
}

func (l Logger) Fatal(msg string) {
	if exitFunction == nil {
		write(l.zlog.Fatal(), msg)
	} else {
		write(l.zlog.Error(), msg)
		exitFunction()
	}
}

func (l Logger) FatalE(err error, msg string) {
	if exitFunction == nil {
		writeE(l.zlog.Fatal(), err, msg)
	} else {
		writeE(l.zlog.Error(), err, msg)
		exitFunction()
	}
}

func (l Logger) FatalF(format string, v ...interface{}) {
	if exitFunction == nil {
		writeF(l.zlog.Fatal(), format, v...)
	} else {
		writeF(l.zlog.Error(), format, v...)
		exitFunction()
	}
}

func (l Logger) FatalEF(err error, format string, v ...interface{}) {
	if exitFunction == nil {
		writeEF(l.zlog.Fatal(), err, format, v...)
	} else {
		writeEF(l.zlog.Error(), err, format, v...)
		exitFunction()
	}
}

///////////////////// private helper functions

func write(e *zerolog.Event, msg string) {
	e.Msg(msg)
}

func writeT(e *zerolog.Event, t Tag, msg string) {
	t.apply(e)
	e.Msg(msg)
}

func writeF(e *zerolog.Event, format string, v ...interface{}) {
	e.Msgf(format, v...)
}

func writeTF(e *zerolog.Event, t Tag, format string, v ...interface{}) {
	t.apply(e)
	e.Msgf(format, v...)
}

func writeE(e *zerolog.Event, err error, msg string) {
	e.Err(err).Msg(msg)
}

func writeEF(e *zerolog.Event, err error, format string, v ...interface{}) {
	e.Err(err).Msgf(format, v...)
}

///////////////////// Tags

type tagInt64 struct {
	key   string
	value int64
}

func (t tagInt64) apply(logEvent *zerolog.Event) {
	logEvent.Int64(t.key, t.value)
}

type tagInt struct {
	key   string
	value int
}

func (t tagInt) apply(logEvent *zerolog.Event) {
	logEvent.Int(t.key, t.value)
}

type tagStr struct {
	key   string
	value string
}

func (t tagStr) apply(logEvent *zerolog.Event) {
	logEvent.Str(t.key, t.value)
}

type tagFloat32 struct {
	key   string
	value float32
}

func (t tagFloat32) apply(logEvent *zerolog.Event) {
	logEvent.Float32(t.key, t.value)
}

type tagFloat64 struct {
	key   string
	value float64
}

func (t tagFloat64) apply(logEvent *zerolog.Event) {
	logEvent.Float64(t.key, t.value)
}

type tagErr struct {
	err error
}

func (t tagErr) apply(logEvent *zerolog.Event) {
	logEvent.Err(t.err)
}

type multiTag struct {
	tags []Tag
}

func (x multiTag) apply(logEvent *zerolog.Event) {
	for _, v := range x.tags {
		v.apply(logEvent)
	}
}
