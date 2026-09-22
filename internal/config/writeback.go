package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// EndpointField is a value hookly fills in for one endpoint of hookly.yaml,
// such as the id the edge gave an endpoint it just created.
type EndpointField struct {
	Endpoint int    // Index in endpoints:
	Key      string // "id" or "url"
	Value    string
}

// keyAfter is where a missing key goes: after the first of these keys the
// endpoint has, else first.
var keyAfter = map[string][]string{
	"id":  {"name"},
	"url": {"id", "name"},
}

// SetEndpointFields writes fields into the hookly.yaml at path and replaces
// the file atomically (temp file + rename) so it is never left half-written.
// Only the lines holding those fields change: comments, blank lines and
// quoting elsewhere stay byte for byte.
func SetEndpointFields(path string, fields []EndpointField) error {
	if len(fields) == 0 {
		return nil
	}
	// Write through a symlink rather than replacing it
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, f := range fields {
		if data, err = setEndpointField(data, f); err != nil {
			return err
		}
	}
	return writeFileAtomic(path, data, info.Mode().Perm())
}

// setEndpointField edits one field in the text of a hookly.yaml, locating it
// with the parser's line and column positions.
func setEndpointField(data []byte, f EndpointField) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse hookly.yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("hookly.yaml is not a mapping")
	}
	endpoints := mappingValue(doc.Content[0], "endpoints")
	if endpoints == nil || endpoints.Kind != yaml.SequenceNode {
		return nil, errors.New("hookly.yaml has no endpoints list")
	}
	if f.Endpoint < 0 || f.Endpoint >= len(endpoints.Content) {
		return nil, fmt.Errorf("endpoint %d not found in hookly.yaml", f.Endpoint)
	}
	m := endpoints.Content[f.Endpoint]
	if m.Kind != yaml.MappingNode || m.Style&yaml.FlowStyle != 0 || len(m.Content) == 0 {
		return nil, fmt.Errorf("endpoint %d in hookly.yaml is not a block mapping", f.Endpoint)
	}

	lines := strings.SplitAfter(string(data), "\n")
	quoted := strconv.Quote(f.Value)

	// Replace an existing value in place, keeping any comment after it
	if v := mappingValue(m, f.Key); v != nil {
		if v.Kind != yaml.ScalarNode || v.Line < 1 || v.Line > len(lines) {
			return nil, fmt.Errorf("endpoint %d: %s is not a plain value", f.Endpoint, f.Key)
		}
		line := lines[v.Line-1]
		eol := line[len(strings.TrimRight(line, "\r\n")):]
		text := line[:v.Column-1] + quoted
		if v.LineComment != "" {
			text += " " + v.LineComment
		}
		lines[v.Line-1] = text + eol
		return []byte(strings.Join(lines, "")), nil
	}

	// Insert a new line: after the first anchor key present, else first
	first := m.Content[0]
	indent := strings.Repeat(" ", first.Column-1)
	for _, after := range keyAfter[f.Key] {
		if k, v := mappingKey(m, after); k != nil && v.Kind == yaml.ScalarNode && v.Line == k.Line {
			return []byte(insertLine(lines, k.Line, indent+f.Key+": "+quoted)), nil
		}
	}
	// Before the first key. On a "- key:" line the new key takes the dash.
	line := lines[first.Line-1]
	if first.Column >= 3 && line[first.Column-3:first.Column-1] == "- " {
		lines[first.Line-1] = line[:first.Column-1] + f.Key + ": " + quoted + "\n" + indent + line[first.Column-1:]
		return []byte(strings.Join(lines, "")), nil
	}
	return []byte(insertLine(lines, first.Line-1, indent+f.Key+": "+quoted)), nil
}

// insertLine inserts text as a new line after line number after (1-based; 0
// inserts at the top).
func insertLine(lines []string, after int, text string) string {
	if after > 0 && !strings.HasSuffix(lines[after-1], "\n") {
		lines[after-1] += "\n"
	}
	out := append([]string{}, lines[:after]...)
	out = append(out, text+"\n")
	out = append(out, lines[after:]...)
	return strings.Join(out, "")
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	_, v := mappingKey(m, key)
	return v
}

func mappingKey(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
