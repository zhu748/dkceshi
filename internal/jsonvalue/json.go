// Package jsonvalue decodes JSON without rounding numbers that float64 cannot preserve.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
)

func Unmarshal(data []byte, dst any) error { return Decode(bytes.NewReader(data), dst) }

func Decode(r io.Reader, dst any) error {
	d := json.NewDecoder(r)
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected trailing JSON value")
	}
	switch v := dst.(type) {
	case *any:
		*v = normalize(*v)
	case *map[string]any:
		normalize(*v)
	case *[]any:
		normalize(*v)
	}
	return nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			x[k] = normalize(value)
		}
	case []any:
		for i, value := range x {
			x[i] = normalize(value)
		}
	case json.Number:
		if len(x) > 128 {
			return x
		}
		if e := strings.IndexAny(string(x), "eE"); e >= 0 {
			exponent, err := strconv.ParseInt(string(x)[e+1:], 10, 32)
			if err != nil || exponent > 400 || exponent < -400 {
				return x
			}
		}
		// Preserve the established float64 representation for exactly round-trippable
		// values; retain json.Number whenever encoding the float would change its value.
		f, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return x
		}
		before, ok := new(big.Rat).SetString(string(x))
		after, valid := new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, 64))
		if ok && valid && before.Cmp(after) == 0 {
			return f
		}
	}
	return v
}
