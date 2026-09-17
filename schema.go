package barista

import (
	_ "embed"
	"fmt"
	"sort"
)

//go:embed schemas/repos.schema.json
var reposSchema []byte

//go:embed schemas/config.schema.json
var configSchema []byte

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
		Description: "global personal settings (<workspace>/.barista/config.json)",
		Raw:         configSchema,
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
