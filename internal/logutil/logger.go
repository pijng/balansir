package logutil

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	tagInfo    = "INFO    "
	tagNotice  = "NOTICE  "
	tagWarning = "WARNING "
	tagError   = "ERROR   "
	tagFatal   = "FATAL   "
)

const (
	infoColor    = "\033[1;37m%s\033[0m"
	noticeColor  = "\033[1;32m%s\033[0m"
	warningColor = "\033[1;33m%s\033[0m"
	errorColor   = "\033[1;31m%s\033[0m"
	fatalColor   = "\033[1;35m%s\033[0m"
)

const (
	logDir      = "./log"
	jsonDir     = "./log/.dashboard"
	logPrefix   = "balansir"
	jsonPrefix  = "logs"
	statsPrefix = "stats"
	logPath     = "./log/balansir.log"
	//JSONPath ...
	JSONPath = "./log/.dashboard/logs.json"
	//StatsPath ...
	StatsPath = "./log/.dashboard/stats.json"
)

const (
	logMaxSize = 100 * 1024 * 1024
)

//JSONlog ...
type JSONlog struct {
	Timestamp time.Time `json:"timestamp"`
	Tag       string    `json:"tag"`
	Text      string    `json:"text"`
}

//Logger ...
type Logger struct {
	logger      *log.Logger
	logFile     *os.File
	jsonFile    *os.File
	statsFile   *os.File
	jsonBuffer  []byte
	statsBuffer []byte
	mux         sync.RWMutex
}

var defaultLogger *Logger
var colors map[string]string

//Init ...
func Init() {
	colors = map[string]string{
		tagInfo:    infoColor,
		tagNotice:  noticeColor,
		tagWarning: warningColor,
		tagError:   errorColor,
		tagFatal:   fatalColor,
	}

	ensureDirExist(logDir)
	ensureDirExist(jsonDir)

	jsonFile, _ := openOrCreateFile(JSONPath)
	statsFile, _ := openOrCreateFile(StatsPath)
	logFile, _ := openOrCreateFile(logPath)

	logger := log.New(logFile, "", 0)

	defaultLogger = &Logger{
		logger:    logger,
		logFile:   logFile,
		jsonFile:  jsonFile,
		statsFile: statsFile,
	}
}

func ensureDirExist(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			log.Fatalf("failed to create %s directory: %v", path, err)
		}
	}
}

func (l *Logger) ensureFileExist(path string) {
	file, new := openOrCreateFile(path)

	if new {
		switch path {
		case logPath:
			l.logger = log.New(file, "", 0)
			l.logFile = file
		case JSONPath:
			l.jsonFile = file
		case StatsPath:
			l.statsFile = file
		}
	}
}

func openOrCreateFile(path string) (*os.File, bool) {
	_, errNotExist := os.Stat(path)
	file, err := os.OpenFile(path, getFileFlags(path), 0660)

	if os.IsNotExist(errNotExist) {
		if err != nil {
			log.Fatalf("failed to create/open %s file: %v", path, err)
			return nil, false
		}

		return file, true
	}

	return file, false
}

func (l *Logger) output(severity string, txt string) {
	l.mux.Lock()
	defer l.mux.Unlock()

	l.log(severity, txt)
	l.logJSON(time.Now(), severity, txt)
}

func (l *Logger) log(severity string, txt string) {
	l.ensureFileExist(logPath)

	if fileSize(l.logFile) > logMaxSize {
		l.moveLog()
		l.ensureFileExist(logPath)
	}

	l.logger.Output(3, logFormat(colors[severity], dateFormat(time.Now()), severity, txt))
}

func (l *Logger) logJSON(cTime time.Time, tag string, txt string) {
	info, err := l.jsonFile.Stat()
	if err != nil {
		l.malformedJSON(err)
		return
	}
	length := info.Size()

	if length == 0 {
		_, err = l.jsonFile.WriteAt([]byte("[]"), 0)
		length = 2
		if err != nil {
			Warning(err)
			return
		}
	}

	// trim tag's trailing spaces – we use them in a standard stdout to show logs in a
	// consistent way. On the frontend we use table columns and styles to create that consistency.
	tag = strings.TrimSpace(tag)
	jsonLog := JSONlog{Timestamp: cTime, Tag: tag, Text: txt}

	l.jsonBuffer, err = json.Marshal(jsonLog)
	if err != nil {
		l.malformedJSON(err)
		return
	}

	if length > 2 {
		l.jsonBuffer = append([]byte(","), l.jsonBuffer...)
	}
	l.jsonBuffer = append(l.jsonBuffer, []byte("]")...)

	_, err = l.jsonFile.WriteAt(l.jsonBuffer, length-1)
	if err != nil {
		Warning(err)
	}
}

func (l *Logger) stats(stats interface{}) {
	l.mux.Lock()
	defer l.mux.Unlock()

	info, err := l.statsFile.Stat()
	if err != nil {
		l.malformedJSON(err)
		return
	}
	length := info.Size()

	if length == 0 {
		_, err = l.statsFile.WriteAt([]byte("[]"), 0)
		length = 2
		if err != nil {
			Warning(err)
			return
		}
	}

	l.statsBuffer, err = json.Marshal(stats)
	if err != nil {
		l.malformedJSON(err)
		return
	}

	if length > 2 {
		l.statsBuffer = append([]byte(","), l.statsBuffer...)
	}
	l.statsBuffer = append(l.statsBuffer, []byte("]")...)

	_, err = l.statsFile.WriteAt(l.statsBuffer, length-1)
	if err != nil {
		Warning(err)
	}
}

func (l *Logger) malformedJSON(err error) {
	l.logger.Output(3, logFormat(warningColor, dateFormat(time.Now()), tagWarning, fmt.Sprintf("%s malformed: %v", JSONPath, err))) //nolint
}

func (l *Logger) moveLog() {
	defer l.logFile.Close()

	newName := fmt.Sprintf("./%s/%s-%v.log", logDir, logPrefix, time.Now().Unix())
	err := os.Rename(l.logFile.Name(), newName)

	if err != nil {
		log.Fatalf("failed to rename %s file: %v", l.logFile.Name(), err)
	}
}

func fileSize(file *os.File) int64 {
	stat, err := file.Stat()
	if err != nil {
		log.Fatalf("failed to read %s info: %v", file.Name(), err)
	}

	return stat.Size()
}

func getFileFlags(path string) int {
	if path == JSONPath || path == StatsPath {
		return os.O_CREATE | os.O_RDWR
	}
	return os.O_CREATE | os.O_WRONLY | os.O_APPEND
}

//Info ...
func Info(txt interface{}) {
	defaultLogger.output(tagInfo, fmt.Sprint(txt))
}

//Notice ...
func Notice(txt interface{}) {
	defaultLogger.output(tagNotice, fmt.Sprint(txt))
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
func Fatal(txt interface{}) {
	defaultLogger.output(tagFatal, fmt.Sprint(txt))
}

//Stats ...
func Stats(stats interface{}) {
	defaultLogger.stats(stats)
}
