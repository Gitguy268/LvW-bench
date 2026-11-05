package schema

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
)

func itoa(i int) string { return strconv.Itoa(i) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// orderedMap decodes a JSON object and keeps its key order.
type orderedMap struct{ keys []string }

func (o *orderedMap) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil { // {
		return err
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		k, _ := t.(string)
		o.keys = append(o.keys, k)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return err
		}
	}
	return nil
}
