package common

import (
	"bufio"
	"io"
	"maps"
	"os"
	"strings"
)

// very, very simple properties file handling, doesn't support escaping, etc., just comments and name=value

type Properties interface {
	GetString(key string, def string) string
	SetString(key string, value string)
	Clone() Properties
}

type properties struct {
	props map[string]string
}

func (p properties) GetString(key string, def string) string {
	key = strings.TrimSpace(key)
	v, ok := p.props[key]
	if !ok {
		return def
	}
	return v
}

func (p properties) SetString(key string, value string) {
	key = strings.TrimSpace(key)
	p.props[key] = value
}

func (p properties) Clone() Properties {
	return properties{props: maps.Clone(p.props)}
}

func NewProperties(file string) (Properties, error) {
	inputFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer inputFile.Close()

	return NewPropertiesFromReader(inputFile)
}

func NewPropertiesFromReader(r io.Reader) (Properties, error) {
	p := properties{props: make(map[string]string)}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s := scanner.Text()
		if strings.HasPrefix(s, "//") || strings.HasPrefix(s, "#") {
			continue
		}
		parts := strings.Split(s, "=")
		if len(parts) != 2 {
			continue
		}
		p.props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return &p, nil
}
