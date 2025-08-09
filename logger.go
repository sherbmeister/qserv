package main

import (
	"log"
	"strings"
)

type Logger struct{ level int }

const (
	lvDebug = iota
	lvInfo
	lvWarn
	lvError
)

func NewLogger(level string) *Logger {
	switch strings.ToLower(level) {
	case "debug":
		return &Logger{level: lvDebug}
	case "warn":
		return &Logger{level: lvWarn}
	case "error":
		return &Logger{level: lvError}
	default:
		return &Logger{level: lvInfo}
	}
}

func (l *Logger) Debugf(f string, a ...any) {
	if l.level <= lvDebug {
		log.Printf("[debug] "+f, a...)
	}
}
func (l *Logger) Infof(f string, a ...any) {
	if l.level <= lvInfo {
		log.Printf("[info ] "+f, a...)
	}
}
func (l *Logger) Warnf(f string, a ...any) {
	if l.level <= lvWarn {
		log.Printf("[warn ] "+f, a...)
	}
}
func (l *Logger) Errorf(f string, a ...any) {
	if l.level <= lvError {
		log.Printf("[error] "+f, a...)
	}
}
func (l *Logger) Fatalf(f string, a ...any) { log.Fatalf("[fatal] "+f, a...) }
