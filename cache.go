package main

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	prog := filepath.Base(os.Args[0])
	dir := filepath.Join(os.ExpandEnv("${HOME}"), ".cache", prog)
	os.MkdirAll(dir, 0755)

	expiry := 900 // default to 15 minutes
	if len(os.Args) > 1 {
		if e, err := strconv.Atoi(os.Args[1]); err == nil {
			expiry = e
		}
	}

	cmd := ""
	if len(os.Args) > 2 {
		cmd = os.Args[2]
	}

	hash := fmt.Sprintf("%x", md5.Sum([]byte(cmd)))
	cacheFile := filepath.Join(dir, hash)

	if _, err := os.Stat(cacheFile); err == nil && time.Since(fileModTime(cacheFile)) <= time.Duration(expiry)*time.Second {
		fmt.Println("true")
	} else {
		fmt.Println("false")
		if _, err := os.Stat(cacheFile); err != nil || time.Since(fileModTime(cacheFile)) > time.Duration(expiry)*time.Second {
			_, _ = os.Create(cacheFile)
			_, _ = os.Exec(os.Args[2], []string{})
		}
	}
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
