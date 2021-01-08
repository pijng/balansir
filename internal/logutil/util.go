package logutil

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func logFormat(color string, txt ...string) string {
	return fmt.Sprintf(color, strings.Join(txt, " "))
}

func dateFormat(cTime time.Time) string {
	dateStamp := cTime.Format("2006/01/02")
	timestamp := cTime.Format("15:04:05")

	return fmt.Sprintf("%v %v ", dateStamp, timestamp)
}

func getFileFlags(path string) int {
	if path == JSONPath || path == StatsPath {
		return os.O_CREATE | os.O_RDWR
	}
	return os.O_CREATE | os.O_WRONLY | os.O_APPEND
}

func fileSize(file *os.File) int64 {
	stat, err := file.Stat()
	if err != nil {
		log.Fatalf("failed to read %s info: %v", file.Name(), err)
	}

	return stat.Size()
}
