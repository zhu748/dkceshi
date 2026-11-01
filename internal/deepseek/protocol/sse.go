package protocol

import (
	"bufio"
	"ds2api/internal/streambudget"
	"io"
	"net/http"
)

func ScanSSELines(resp *http.Response, onLine func([]byte) bool) error {
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	remaining := streambudget.Value("max_output_bytes")
	maxLine := streambudget.Value("max_line_bytes")
	for {
		line, err := streambudget.ReadLine(reader, maxLine)
		if len(line) > remaining {
			return streambudget.ErrLimit
		}
		remaining -= len(line)
		if len(line) > 0 {
			if !onLine(line) {
				return nil
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
