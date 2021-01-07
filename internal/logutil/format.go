package logutil

import (
	"fmt"
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
