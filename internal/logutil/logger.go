package logutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	logDir  = "./log"
	logPath = "./log/balansir.log"
	jsonDir = "./log/.dashboard"
	//JSONPath ...
	JSONPath = "./log/.dashboard/logs.json"
	//StatsPath ...
	StatsPath = "./log/.dashboard/stats.json"
)

//JSONlog ...
type JSONlog struct {
	Timestamp time.Time `json:"timestamp"`
	Tag       string    `json:"tag"`
	Text      string    `json:"text"`
}

//Logger ...
type Logger struct {
	logger *log.Logger
	mux    sync.RWMutex
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

	ensureDirExist(rootedPath(logDir))
	ensureDirExist(rootedPath(jsonDir))

	openOrCreateFile(rootedPath(JSONPath))
	openOrCreateFile(rootedPath(StatsPath))

	lf, _ := openOrCreateFile(rootedPath(logPath))
	logger := log.New(lf, "", 0)

	defaultLogger = &Logger{
		logger: logger,
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
	lf, new := openOrCreateFile(path)

	if new && path == logPath {
		l.logger = log.New(lf, "", 0)
	}
}

func openOrCreateFile(path string) (*os.File, bool) {
	_, errNotExist := os.Stat(path)
	lf, err := os.OpenFile(path, getFileFlags(path), 0660)

	if os.IsNotExist(errNotExist) {
		if err != nil {
			log.Fatalf("failed to create/open %s file: %v", path, err)
			return nil, false
		}

		return lf, true
	}

	return lf, false
}

func (l *Logger) output(severity string, txt string) {
	l.mux.Lock()
	defer l.mux.Unlock()

	l.ensureFileExist(logPath)

	l.logger.Output(3, logFormat(colors[severity], dateFormat(time.Now()), severity, txt))
	l.jsonLog(time.Now(), severity, txt)
}

func (l *Logger) jsonLog(cTime time.Time, tag string, txt string) {
	file, _ := openOrCreateFile(JSONPath)
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		l.malformedJSON(err)
		return
	}
	if len(bytes) == 0 {
		bytes = []byte("[]")
	}

	var jsonLogs []JSONlog
	err = json.Unmarshal(bytes, &jsonLogs)
	if err != nil {
		l.malformedJSON(err)
		return
	}

	// trim tag's trailing spaces – we use them in a standard stdout to show logs in a
	// consistent way. On the frontend we use table columns and styles to create that consistency.
	tag = strings.TrimSpace(tag)
	jsonLogs = append(jsonLogs, JSONlog{Timestamp: cTime, Tag: tag, Text: txt})
	newBytes, err := json.Marshal(jsonLogs)
	if err != nil {
		l.malformedJSON(err)
		return
	}

	_, err = file.WriteAt(newBytes, 0)
	if err != nil {
		l.malformedJSON(err)
		return
	}
}

var jsonLogs []byte
var sFile *os.File
var sErr error

func (l *Logger) stats(stats interface{}) {
	l.mux.Lock()
	defer l.mux.Unlock()

	sFile, _ = openOrCreateFile(StatsPath)
	defer sFile.Close()

	info, err := sFile.Stat()
	if err != nil {
		l.malformedJSON(err)
		return
	}
	length := info.Size()

	if length == 0 {
		_, err = sFile.WriteAt([]byte("[]"), 0)
		length = 2
		if err != nil {
			Warning(err)
			return
		}
	}

	jsonLogs, err = json.Marshal(stats)
	if err != nil {
		l.malformedJSON(err)
		return
	}

	if length > 2 {
		jsonLogs = append([]byte(","), jsonLogs...)
	}
	jsonLogs = append(jsonLogs, []byte("]")...)

	_, err = sFile.WriteAt(jsonLogs, length-1)
	if err != nil {
		Warning(err)
	}
}

func (l *Logger) malformedJSON(err error) {
	l.logger.Output(3, logFormat(warningColor, dateFormat(time.Now()), tagWarning, fmt.Sprintf("%s malformed: %v", JSONPath, err))) //nolint
}

func getFileFlags(path string) int {
	if path == JSONPath || path == StatsPath {
		return os.O_CREATE | os.O_RDWR
	}
	return os.O_CREATE | os.O_WRONLY | os.O_APPEND
}

func rootedPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get root path: %v", err)
	}
	return fmt.Sprintf("%s/%s", wd, path)
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
