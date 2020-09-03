package pstats

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//GetRSSInfoLinux ...
func GetRSSInfoLinux() (int64, error) {
	pid := os.Getpid()
	memPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "statm")

	contents, err := ioutil.ReadFile(memPath)
	if err != nil {
		return 0, err
	}

	fields := strings.Split(string(contents), " ")

	rss, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}

	return int64(int(rss)*os.Getpagesize()) / 1024 / 1024, nil
}
