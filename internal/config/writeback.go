package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// SetEndpointFields writes fields into the hookly.yaml at path, keeping its
// comments and layout, and replaces the file atomically (temp file + rename)
// so it is never left half-written.
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

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("hookly.yaml is not a mapping")
	}
	endpoints := mappingValue(doc.Content[0], "endpoints")
	if endpoints == nil || endpoints.Kind != yaml.SequenceNode {
		return errors.New("hookly.yaml has no endpoints list")
	}
	for _, f := range fields {
		if f.Endpoint < 0 || f.Endpoint >= len(endpoints.Content) || endpoints.Content[f.Endpoint].Kind != yaml.MappingNode {
			return fmt.Errorf("endpoint %d not found in hookly.yaml", f.Endpoint)
		}
		setMappingValue(endpoints.Content[f.Endpoint], f.Key, f.Value)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes(), info.Mode().Perm())
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMappingValue(m *yaml.Node, key, value string) {
	if v := mappingValue(m, key); v != nil {
		v.Kind, v.Tag, v.Value = yaml.ScalarNode, "!!str", value
		return
	}
	at := 0
	for _, after := range keyAfter[key] {
		found := false
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == after {
				at, found = i+2, true
				break
			}
		}
		if found {
			break
		}
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle}
	m.Content = append(m.Content[:at], append([]*yaml.Node{k, v}, m.Content[at:]...)...)
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
