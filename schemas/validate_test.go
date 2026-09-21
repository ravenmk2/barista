package schemas

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		doc       string
		wantValid bool
		wantPaths []string
	}{
		{
			name:      "repos minimal valid",
			schema:    "repos",
			doc:       `{"baseUrl":"git@github.com:your-org/","repos":[{"name":"a","url":"a"}]}`,
			wantValid: true,
		},
		{
			name:      "repos missing name",
			schema:    "repos",
			doc:       `{"repos":[{"url":"a"}]}`,
			wantPaths: []string{"repos[0]"},
		},
		{
			name:      "repos url wrong type",
			schema:    "repos",
			doc:       `{"repos":[{"name":"a","url":42}]}`,
			wantPaths: []string{"repos[0].url"},
		},
		{
			name:      "repos labels wrong type",
			schema:    "repos",
			doc:       `{"repos":[{"name":"a","url":"a","labels":"backend"}]}`,
			wantPaths: []string{"repos[0].labels"},
		},
		{
			name:      "repos path dotdot escape",
			schema:    "repos",
			doc:       `{"repos":[{"name":"a","url":"a","path":"../escape"}]}`,
			wantPaths: []string{"repos[0].path"},
		},
		{
			name:      "repos path absolute",
			schema:    "repos",
			doc:       `{"repos":[{"name":"a","url":"a","path":"/abs/x"}]}`,
			wantPaths: []string{"repos[0].path"},
		},
		{
			name:      "config empty object valid",
			schema:    "config",
			doc:       `{}`,
			wantValid: true,
		},
		{
			name:      "config parallel wrong type",
			schema:    "config",
			doc:       `{"parallel":"ten"}`,
			wantPaths: []string{"parallel"},
		},
		{
			name:      "config color invalid enum",
			schema:    "config",
			doc:       `{"color":"rainbow"}`,
			wantPaths: []string{"color"},
		},
		{
			name:      "config colorProfile invalid enum",
			schema:    "config",
			doc:       `{"colorProfile":"8bit"}`,
			wantPaths: []string{"colorProfile"},
		},
		{
			name:      "config colorProfile valid",
			schema:    "config",
			doc:       `{"color":"always","colorProfile":"256"}`,
			wantValid: true,
		},
		{
			name:      "properties scalar values valid",
			schema:    "properties",
			doc:       `{"jdk":"17","maven.default":"maven-3.9","gradle.default":"gradle-8.10","gradle.user.home":".barista/gradle-home","git.fetch.prune":true,"git.pull.rebase":false,"threads":4,"future.key":"x"}`,
			wantValid: true,
		},
		{
			name:      "properties non-scalar value",
			schema:    "properties",
			doc:       `{"jdk":["17"]}`,
			wantPaths: []string{"jdk"},
		},
		{
			name:      "maven minimal valid",
			schema:    "maven",
			doc:       `{"installations":[{"name":"maven-3.9","version":"3.9.11","path":"/opt/maven"}],"default":"maven-3.9","jdk":"17"}`,
			wantValid: true,
		},
		{
			name:      "maven empty object valid",
			schema:    "maven",
			doc:       `{}`,
			wantValid: true,
		},
		{
			name:      "maven installation missing version",
			schema:    "maven",
			doc:       `{"installations":[{"name":"maven-3.9","path":"/opt/maven"}]}`,
			wantPaths: []string{"installations[0]"},
		},
		{
			name:      "maven bad name pattern",
			schema:    "maven",
			doc:       `{"installations":[{"name":"Maven-3.9","version":"3.9.11","path":"/opt/maven"}]}`,
			wantPaths: []string{"installations[0].name"},
		},
		{
			name:      "gradle minimal valid",
			schema:    "gradle",
			doc:       `{"installations":[{"name":"gradle-8.10","version":"8.10.2","path":"/opt/gradle"}],"default":"gradle-8.10","jdk":"17"}`,
			wantValid: true,
		},
		{
			name:      "gradle empty object valid",
			schema:    "gradle",
			doc:       `{}`,
			wantValid: true,
		},
		{
			name:      "gradle installation missing version",
			schema:    "gradle",
			doc:       `{"installations":[{"name":"gradle-8.10","path":"/opt/gradle"}]}`,
			wantPaths: []string{"installations[0]"},
		},
		{
			name:      "gradle bad name pattern",
			schema:    "gradle",
			doc:       `{"installations":[{"name":"Gradle-8.10","version":"8.10.2","path":"/opt/gradle"}]}`,
			wantPaths: []string{"installations[0].name"},
		},
		{
			name:      "manifest minimal valid",
			schema:    "manifest",
			doc:       `{"schemaVersion":1,"version":"v0.3.0","assets":{"windows/amd64":{"file":"barista-windows-amd64.exe","sha256":"` + strings.Repeat("a", 64) + `","size":10}}}`,
			wantValid: true,
		},
		{
			name:      "manifest missing assets",
			schema:    "manifest",
			doc:       `{"schemaVersion":1,"version":"v0.3.0"}`,
			wantPaths: []string{"$"},
		},
		{
			name:      "manifest bad sha256",
			schema:    "manifest",
			doc:       `{"schemaVersion":1,"version":"v0.3.0","assets":{"linux/amd64":{"file":"barista-linux-amd64","sha256":"xyz","size":10}}}`,
			wantPaths: []string{"assets.linux.amd64.sha256"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues, err := Validate(tc.schema, []byte(tc.doc))
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if tc.wantValid {
				if len(issues) != 0 {
					t.Fatalf("want valid, got issues: %+v", issues)
				}
				return
			}
			if len(issues) == 0 {
				t.Fatal("want invalid, got no issues")
			}
			got := map[string]bool{}
			for _, i := range issues {
				got[i.Path] = true
			}
			for _, p := range tc.wantPaths {
				if !got[p] {
					t.Errorf("missing issue at path %q; got %+v", p, issues)
				}
			}
		})
	}
}

func TestValidateSyntaxError(t *testing.T) {
	issues, err := Validate("repos", []byte(`{ "repos": [ broken`))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %+v", issues)
	}
	if issues[0].Path != "$" {
		t.Errorf("issue path = %q, want $", issues[0].Path)
	}
	if !strings.Contains(issues[0].Message, "invalid JSON") {
		t.Errorf("issue message = %q, want to contain 'invalid JSON'", issues[0].Message)
	}
}

func TestValidateUnknownSchema(t *testing.T) {
	if _, err := Validate("nope", []byte(`{}`)); err == nil {
		t.Error("want error for unknown schema, got nil")
	}
}
