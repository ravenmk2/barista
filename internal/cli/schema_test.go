package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runSchema(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	oldCode := ExitCode
	ExitCode = 0
	defer func() { ExitCode = oldCode }()

	cmd := schemaCmd()
	cmd.SetArgs(args)
	_ = cmd.Execute()

	wOut.Close()
	wErr.Close()
	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)
	return string(out), string(errOut), ExitCode
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type validateOutput struct {
	Schema string  `json:"schema"`
	File   *string `json:"file"`
	Valid  bool    `json:"valid"`
	Errors []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"errors"`
}

func parseValidate(t *testing.T, stdout string) validateOutput {
	t.Helper()
	var r validateOutput
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	return r
}

func TestSchemaListEquivalence(t *testing.T) {
	out1, _, code1 := runSchema(t)
	out2, _, code2 := runSchema(t, "list")
	if code1 != 0 || code2 != 0 {
		t.Fatalf("exit codes: schema=%d list=%d, want 0", code1, code2)
	}
	if out1 != out2 {
		t.Fatalf("`schema` and `schema list` output differ:\n%s\n---\n%s", out1, out2)
	}
	for _, name := range []string{"config", "repos"} {
		if !strings.Contains(out1, name) {
			t.Errorf("list output missing %q", name)
		}
	}
}

func TestSchemaShowRepos(t *testing.T) {
	disk, err := os.ReadFile(filepath.Join("..", "..", "schemas", "repos.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runSchema(t, "show", "repos")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout != string(disk) {
		t.Error("show repos output differs from disk file")
	}
}

func TestSchemaShowUnknown(t *testing.T) {
	_, stderr, code := runSchema(t, "show", "nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown schema") {
		t.Errorf("stderr missing 'unknown schema': %s", stderr)
	}
}

func TestSchemaValidateWorkspaceRepos(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".barista", "repos.json"),
		`{"baseUrl":"git@github.com:your-org/","repos":[{"name":"a","url":"a"}]}`)
	t.Chdir(dir)

	stdout, _, code := runSchema(t, "validate", "repos")
	r := parseValidate(t, stdout)
	if code != 0 || !r.Valid || r.File == nil {
		t.Fatalf("good config: code=%d valid=%v file=%v", code, r.Valid, r.File)
	}

	writeFile(t, filepath.Join(dir, ".barista", "repos.json"), `{"repos":[{"url":"a"}]}`)
	stdout, _, code = runSchema(t, "validate", "repos")
	r = parseValidate(t, stdout)
	if code != 1 || r.Valid {
		t.Fatalf("bad config: code=%d valid=%v, want code=1 valid=false", code, r.Valid)
	}
	if len(r.Errors) == 0 {
		t.Error("bad config: want errors")
	}
}

func TestSchemaValidateConfigMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".barista", "repos.json"), `{"repos":[{"name":"a","url":"a"}]}`)
	t.Chdir(dir)

	stdout, _, code := runSchema(t, "validate", "config")
	r := parseValidate(t, stdout)
	if code != 0 || !r.Valid || r.File != nil {
		t.Fatalf("code=%d valid=%v file=%v, want code=0 valid=true file=null", code, r.Valid, r.File)
	}
}

func TestSchemaValidateExplicitFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	writeFile(t, good, `{"repos":[{"name":"a","url":"a"}]}`)
	writeFile(t, bad, `{"repos":[{"url":"a"}]}`)

	stdout, _, code := runSchema(t, "validate", "repos", good)
	if r := parseValidate(t, stdout); code != 0 || !r.Valid {
		t.Fatalf("explicit good file: code=%d valid=%v", code, r.Valid)
	}

	stdout, _, code = runSchema(t, "validate", "repos", bad)
	if r := parseValidate(t, stdout); code != 1 || r.Valid {
		t.Fatalf("explicit bad file: code=%d valid=%v", code, r.Valid)
	}
}

func TestSchemaValidateNoWorkspace(t *testing.T) {
	t.Chdir(t.TempDir())
	_, stderr, code := runSchema(t, "validate", "repos")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "no .barista directory") {
		t.Errorf("stderr: %s", stderr)
	}
}

func TestSchemaValidateUnknownName(t *testing.T) {
	_, stderr, code := runSchema(t, "validate", "nope")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown schema") {
		t.Errorf("stderr missing 'unknown schema': %s", stderr)
	}
}
