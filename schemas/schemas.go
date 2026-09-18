package schemas

import (
	_ "embed"
	"fmt"
	"sort"
)

//go:embed repos.schema.json
var reposSchema []byte

//go:embed config.schema.json
var configSchema []byte

//go:embed jdk.schema.json
var jdkSchema []byte

type Entry struct {
	Name        string
	Description string
	Raw         []byte
}

var entries = map[string]Entry{
	"repos": {
		Name:        "repos",
		Description: "workspace repository manifest (<workspace>/.barista/repos.json)",
		Raw:         reposSchema,
	},
	"config": {
		Name:        "config",
		Description: "barista settings (user level ~/.config/barista/config.json, workspace level <workspace>/.barista/config.json)",
		Raw:         configSchema,
	},
	"jdk": {
		Name:        "jdk",
		Description: "JDK registry (user level ~/.config/barista/jdk.json)",
		Raw:         jdkSchema,
	},
}

func List() []Entry {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Entry, len(names))
	for i, name := range names {
		out[i] = entries[name]
	}
	return out
}

func Get(name string) (Entry, error) {
	e, ok := entries[name]
	if !ok {
		return Entry{}, fmt.Errorf("unknown schema %q", name)
	}
	return e, nil
}
