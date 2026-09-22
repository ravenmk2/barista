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
			name:      "config mirrors valid",
			schema:    "config",
			doc:       `{"download.mirror":"cn","jdk.download.mirror":"tuna","maven.download.mirror":"https://mirrors.example.com/apache","gradle.download.mirror":"huawei","node.download.mirror":"cn"}`,
			wantValid: true,
		},
		{
			name:      "config download.mirror custom URL invalid",
			schema:    "config",
			doc:       `{"download.mirror":"https://mirrors.example.com/apache"}`,
			wantPaths: []string{"download.mirror"},
		},
		{
			name:      "config jdk mirror custom URL invalid",
			schema:    "config",
			doc:       `{"jdk.download.mirror":"https://mirrors.example.com/Adoptium"}`,
			wantPaths: []string{"jdk.download.mirror"},
		},
		{
			name:      "config maven mirror non-https invalid",
			schema:    "config",
			doc:       `{"maven.download.mirror":"http://mirrors.example.com/apache"}`,
			wantPaths: []string{"maven.download.mirror"},
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
			name:      "node minimal valid",
			schema:    "node",
			doc:       `{"installations":[{"name":"node-22.14.0","version":"22.14.0","path":"/opt/node"}],"default":"node-22.14.0"}`,
			wantValid: true,
		},
		{
			name:      "node empty object valid",
			schema:    "node",
			doc:       `{}`,
			wantValid: true,
		},
		{
			name:      "node installation missing version",
			schema:    "node",
			doc:       `{"installations":[{"name":"node-22","path":"/opt/node"}]}`,
			wantPaths: []string{"installations[0]"},
		},
		{
			name:      "node bad name pattern",
			schema:    "node",
			doc:       `{"installations":[{"name":"Node-22","version":"22.14.0","path":"/opt/node"}]}`,
			wantPaths: []string{"installations[0].name"},
		},
		{
			name:      "release minimal valid",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"executable-zip","platforms":["windows/amd64"],"file":"barista-v0.4.0-windows-amd64.zip","entry":"barista.exe","hashes":{"sha256":"` + strings.Repeat("a", 64) + `"},"size":10}]}`,
			wantValid: true,
		},
		{
			name:      "release platform-agnostic asset valid",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"checksums","file":"checksums.txt","hashes":{"sha256":"` + strings.Repeat("a", 64) + `"},"size":10}]}`,
			wantValid: true,
		},
		{
			name:      "release v1 rejected",
			schema:    "release",
			doc:       `{"schemaVersion":1,"version":"v0.3.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"executable-zip","platforms":["windows/amd64"],"file":"barista-windows-amd64.zip","entry":"barista.exe","hashes":{"sha256":"` + strings.Repeat("a", 64) + `"},"size":10}]}`,
			wantPaths: []string{"schemaVersion"},
		},
		{
			name:      "release missing assets",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `"}`,
			wantPaths: []string{"$"},
		},
		{
			name:      "release executable without platforms",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"executable-tgz","file":"barista-v0.4.0-linux-amd64.tar.gz","entry":"barista","hashes":{"sha256":"` + strings.Repeat("a", 64) + `"},"size":10}]}`,
			wantPaths: []string{"assets[0]"},
		},
		{
			name:      "release bad kind pattern",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"Executable_Zip","platforms":["windows/amd64"],"file":"a.zip","entry":"barista.exe","hashes":{"sha256":"` + strings.Repeat("a", 64) + `"},"size":10}]}`,
			wantPaths: []string{"assets[0].kind"},
		},
		{
			name:      "release bad sha256",
			schema:    "release",
			doc:       `{"schemaVersion":2,"version":"v0.4.0","commit":"` + strings.Repeat("a", 40) + `","assets":[{"kind":"executable-tgz","platforms":["linux/amd64"],"file":"barista-v0.4.0-linux-amd64.tar.gz","entry":"barista","hashes":{"sha256":"xyz"},"size":10}]}`,
			wantPaths: []string{"assets[0].hashes.sha256"},
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
