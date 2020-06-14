package logutil

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

const (
	tagInfo    = "INFO    "
	tagWarning = "WARNING "
	tagError   = "ERROR   "
	tagFatal   = "FATAL   "
)

const (
	infoColor    = "\033[1;40m%s\033[0m"
	warningColor = "\033[1;43m%s\033[0m"
	errorColor   = "\033[1;31m%s\033[0m"
	fatalColor   = "\033[1;35m%s\033[0m"
)

const (
	flags   = log.Ldate | log.Ltime
	logDir  = "./log"
	logPath = "./log/data.log"
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
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.Mkdir(logDir, os.ModePerm)
		if err != nil {
			log.Fatalf("failed to create './logs' directory: %v", err)
		}
	}

	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	if err != nil {
		log.Fatalf("failed to create/open log file: %v", err)
	}

	iLogs := log.New(io.MultiWriter([]io.Writer{lf}...), "", flags)
	wLogs := log.New(io.MultiWriter([]io.Writer{lf}...), "", flags)
	eLogs := log.New(io.MultiWriter([]io.Writer{lf}...), "", flags)
	fLogs := log.New(io.MultiWriter([]io.Writer{lf}...), "", flags)

	iLogs.SetPrefix(fmt.Sprintf(infoColor, tagInfo))
	wLogs.SetPrefix(fmt.Sprintf(warningColor, tagWarning))
	eLogs.SetPrefix(fmt.Sprintf(errorColor, tagError))
	fLogs.SetPrefix(fmt.Sprintf(fatalColor, tagFatal))

	defaultLogger = &Logger{
		infoLog:     iLogs,
		warningLog:  wLogs,
		errorLog:    eLogs,
		fatalLog:    fLogs,
		initialized: true,
	}
}

func (l *Logger) output(severity string, txt string, exit ...bool) {
	l.mx.Lock()
	defer l.mx.Unlock()

	switch severity {
	case tagInfo:
		l.infoLog.Output(3, fmt.Sprintf(infoColor, txt))
	case tagWarning:
		l.warningLog.Output(3, fmt.Sprintf(warningColor, txt))
	case tagError:
		l.errorLog.Output(3, fmt.Sprintf(errorColor, txt))
	case tagFatal:
		l.fatalLog.Output(3, fmt.Sprintf(fatalColor, txt))
		if len(exit) > 0 {
			os.Exit(1)
		}
	}
}

//Info ...
func Info(txt interface{}) {
	defaultLogger.output(tagInfo, fmt.Sprint(txt))
}

//Warning ...
func Warning(txt interface{}) {
	defaultLogger.output(tagWarning, fmt.Sprint(txt))
}

//Error ...
func Error(txt interface{}) {
	defaultLogger.output(tagError, fmt.Sprint(txt))
}

//Fatal ...
func Fatal(txt interface{}, exit ...bool) {
	defaultLogger.output(tagFatal, fmt.Sprint(txt), exit...)
}
