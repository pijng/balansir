package pstats

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

//GetRSSInfoDarwin ...
func GetRSSInfoDarwin() (int64, error) {
	pid := os.Getpid()
	bin, err := exec.LookPath("ps")

	if err != nil {
		return 0, err
	}

	var cmd []string
	cmd = []string{"-x", "-o", "rss", "-p", strconv.Itoa(pid)}

	out := exec.Command(bin, cmd...)

	var buf bytes.Buffer
	out.Stdout = &buf
	out.Stderr = &buf

	if err := out.Start(); err != nil {
		return 0, err
	}

	if err := out.Wait(); err != nil {
		return 0, err
	}

	line := strings.Split(string(buf.Bytes()), "\n")
	trimdRss := strings.TrimSpace(strings.Join(line[1:], " "))
	rss, _ := strconv.Atoi(trimdRss)

	return int64(rss) / 1024, nil
}
