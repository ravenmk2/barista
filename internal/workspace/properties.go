package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Properties map[string]any

func (p Properties) String(key string) (string, bool) {
	v, ok := p[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (p Properties) Bool(key string) (bool, bool) {
	v, ok := p[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func LoadProperties(path string) (Properties, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Properties{}, nil
	}
	if err != nil {
		return nil, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(path), err)}
	}
	var props map[string]any
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: %v", filepath.ToSlash(path), err)}
	}
	for k, v := range props {
		switch v.(type) {
		case string, float64, bool:
		default:
			return nil, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: property %q must be a scalar (string, number or boolean)", filepath.ToSlash(path), k)}
		}
	}
	return props, nil
}

func SetProperty(path, key, value string) error {
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(path), err)}
	default:
		if err := json.Unmarshal(data, &doc); err != nil {
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: %v", filepath.ToSlash(path), err)}
		}
	}
	doc[key] = value
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot encode %s: %v", filepath.ToSlash(path), err)}
	}
	out = append(out, '\n')
	return writeFileAtomic(path, out)
}
