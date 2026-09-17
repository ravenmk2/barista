package schemas

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

var compiled = map[string]*jsonschema.Schema{}

func validator(name string) (*jsonschema.Schema, error) {
	if sch, ok := compiled[name]; ok {
		return sch, nil
	}
	e, err := Get(name)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(e.Raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(e.Name+".schema.json", doc); err != nil {
		return nil, err
	}
	sch, err := c.Compile(e.Name + ".schema.json")
	if err != nil {
		return nil, err
	}
	compiled[name] = sch
	return sch, nil
}

func Validate(name string, data []byte) ([]Issue, error) {
	sch, err := validator(name)
	if err != nil {
		return nil, err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return []Issue{{Path: "$", Message: fmt.Sprintf("invalid JSON: %v", err)}}, nil
	}
	if err := sch.Validate(inst); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return nil, err
		}
		issues := []Issue{}
		flattenIssues(ve, &issues)
		if len(issues) == 0 {
			issues = append(issues, Issue{Path: "$", Message: err.Error()})
		}
		return issues, nil
	}
	return nil, nil
}

var printer = message.NewPrinter(language.English)

func flattenIssues(ve *jsonschema.ValidationError, out *[]Issue) {
	if len(ve.Causes) == 0 {
		loc := ""
		if len(ve.InstanceLocation) > 0 {
			loc = "/" + strings.Join(ve.InstanceLocation, "/")
		}
		*out = append(*out, Issue{Path: jsonPath(loc), Message: ve.ErrorKind.LocalizedString(printer)})
		return
	}
	for _, c := range ve.Causes {
		flattenIssues(c, out)
	}
}

func jsonPath(loc string) string {
	if loc == "" {
		return "$"
	}
	var b strings.Builder
	for i, seg := range strings.Split(strings.TrimPrefix(loc, "/"), "/") {
		if _, err := strconv.Atoi(seg); err == nil {
			fmt.Fprintf(&b, "[%s]", seg)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}
