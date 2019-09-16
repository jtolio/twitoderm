package main

import (
	"bufio"
	"io"
	"regexp"
)

var (
	hostRegex = regexp.MustCompile(`^Host:\s+(.*?)$`)
)

func httpHost(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			return "", nil
		}
		host := hostRegex.FindStringSubmatch(text)
		if len(host) > 1 {
			return host[1], nil
		}
	}
	return "", scanner.Err()
}
