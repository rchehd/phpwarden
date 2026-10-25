package agenthook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func unambiguousJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var consume func(int) error
	consume = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("hook settings nesting exceeds limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate or invalid hook settings key")
				}
				seen[name] = true
				if err := consume(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := consume(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected hook settings delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := consume(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("hook settings contain trailing JSON")
	}
	return nil
}
