package monitoring

import (
	"bufio"
	"encoding/json"
	"os"
)

// --- JSONL helpers (generic, reusable) ---

// LoadJSONLFile reads a JSONL file and returns a slice of items.
func LoadJSONLFile[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var items []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, item)
	}
	return items, scanner.Err()
}

// AppendJSONLFile appends a single item as a JSON line to the file.
func AppendJSONLFile[T any](path string, item T) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// RewriteJSONLFile atomically rewrites a JSONL file with the given items.
func RewriteJSONLFile[T any](path string, items []T) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(f)
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		writer.Write(data)
		writer.WriteByte('\n')
	}

	if err := writer.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
