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

//go:embed maven.schema.json
var mavenSchema []byte

//go:embed gradle.schema.json
var gradleSchema []byte

//go:embed properties.schema.json
var propertiesSchema []byte

//go:embed manifest.schema.json
var manifestSchema []byte

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
		Description: "barista settings (user level ~/.barista/config.json, workspace level <workspace>/.barista/config.json)",
		Raw:         configSchema,
	},
	"jdk": {
		Name:        "jdk",
		Description: "JDK registry (user level ~/.barista/jdk.json)",
		Raw:         jdkSchema,
	},
	"maven": {
		Name:        "maven",
		Description: "Maven registry (user level ~/.barista/maven.json)",
		Raw:         mavenSchema,
	},
	"gradle": {
		Name:        "gradle",
		Description: "Gradle registry (user level ~/.barista/gradle.json)",
		Raw:         gradleSchema,
	},
	"properties": {
		Name:        "properties",
		Description: "workspace execution preferences (<workspace>/.barista/properties.json)",
		Raw:         propertiesSchema,
	},
	"manifest": {
		Name:        "manifest",
		Description: "release manifest (GitHub release asset manifest.json, consumed by barista upgrade)",
		Raw:         manifestSchema,
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
