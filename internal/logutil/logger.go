package logutil

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

const (
	tagInfo    = "INFO: "
	tagWarning = "WARNING: "
	tagError   = "ERROR: "
	tagFatal   = "FATAL: "
)

const (
	flags   = log.Ldate | log.Ltime
	logPath = "./logs/data.log"
)

//Logger ...
type Logger struct {
	infoLog     *log.Logger
	warningLog  *log.Logger
	errorLog    *log.Logger
	fatalLog    *log.Logger
	initialized bool
	mx          sync.RWMutex
}

var defaultLogger *Logger

//Init ...
func Init() {
	if _, err := os.Stat("./logs"); os.IsNotExist(err) {
		err := os.Mkdir("./logs", os.ModePerm)
		if err != nil {
			log.Fatalf("failed to create './logs' directory: %v", err)
		}
	}

	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	if err != nil {
		log.Fatalf("failed to create/open log file: %v", err)
	}

	iLogs := []io.Writer{lf}
	wLogs := []io.Writer{lf}
	eLogs := []io.Writer{lf}
	fLogs := []io.Writer{lf}

	defaultLogger = &Logger{
		infoLog:     log.New(io.MultiWriter(iLogs...), tagInfo, flags),
		warningLog:  log.New(io.MultiWriter(wLogs...), tagWarning, flags),
		errorLog:    log.New(io.MultiWriter(eLogs...), tagError, flags),
		fatalLog:    log.New(io.MultiWriter(fLogs...), tagFatal, flags),
		initialized: true,
	}
}

func (l *Logger) output(severity string, txt string) {
	l.mx.Lock()
	defer l.mx.Unlock()

	switch severity {
	case tagInfo:
		l.infoLog.Output(3, txt)
	case tagWarning:
		l.warningLog.Output(3, txt)
	case tagError:
		l.errorLog.Output(3, txt)
	case tagFatal:
		l.fatalLog.Output(3, txt)
		os.Exit(1)
	}
}

//Info ...
func Info(txt ...interface{}) {
	defaultLogger.output(tagInfo, fmt.Sprint(txt...))
}

//Warning ...
func Warning(txt ...interface{}) {
	defaultLogger.output(tagWarning, fmt.Sprint(txt...))
}

//Error ...
func Error(txt ...interface{}) {
	defaultLogger.output(tagError, fmt.Sprint(txt...))
}

//Fatal ...
func Fatal(txt ...interface{}) {
	defaultLogger.output(tagFatal, fmt.Sprint(txt...))
}
