// Package streambudget bounds upstream output independently of display cleanup.
package streambudget

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

//go:embed defaults.json
var defaultsJSON []byte

var defaults = func() map[string]int {
	var values map[string]int
	if err := json.Unmarshal(defaultsJSON, &values); err != nil {
		panic(err)
	}
	return values
}()

var ErrLimit = errors.New("upstream output exceeded configured resource limits")

// Value is shared with Node through defaults.json and identically named envs.
func Value(key string) int {
	value, err := strconv.Atoi(os.Getenv("DS2API_STREAM_" + strings.ToUpper(key)))
	if err != nil || value <= 0 || value > 1<<30 {
		return defaults[key]
	}
	return value
}

// ReadLine never allocates an unbounded ReadBytes buffer for a missing newline.
func ReadLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > limit-len(line) {
			return nil, ErrLimit
		}
		line = append(line, part...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}
