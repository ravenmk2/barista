package nodecli

import (
	"runtime"
	"strings"
	"testing"

	"barista/internal/output"
)

func binSegment(forCmd bool) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	if forCmd {
		return `\bin`
	}
	return "/bin"
}

func TestRenderEnvSh(t *testing.T) {
	got, e := renderEnv("sh", `/opt/node/node 22`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := "export NODE_HOME='/opt/node/node 22'\n" +
		`export PATH="$NODE_HOME` + binSegment(false) + `:$PATH"` + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEnvShEscapesQuote(t *testing.T) {
	got, e := renderEnv("sh", `/opt/it's`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if !strings.Contains(got, `'/opt/it'\''s'`) {
		t.Errorf("single quote not escaped: %q", got)
	}
}

func TestRenderEnvCmd(t *testing.T) {
	got, e := renderEnv("cmd", `C:\Program Files\nodejs22`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := `set "NODE_HOME=C:\Program Files\nodejs22"` + "\r\n" +
		`set "PATH=%NODE_HOME%` + binSegment(true) + `;%PATH%"` + "\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEnvPowerShell(t *testing.T) {
	got, e := renderEnv("powershell", `C:\node's\node-22`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := "$env:NODE_HOME = 'C:\\node''s\\node-22'\n" +
		`$env:PATH = "$env:NODE_HOME` + binSegment(true) + `;$env:PATH"` + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEnvUnsupportedShell(t *testing.T) {
	_, e := renderEnv("fish", "/opt/node")
	if e == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if e.Code != output.CodeUsageError {
		t.Errorf("got code %q, want %q", e.Code, output.CodeUsageError)
	}
}

func TestNormalizeShell(t *testing.T) {
	cases := map[string]string{
		"sh":                                     "sh",
		"bash":                                   "sh",
		"zsh":                                    "sh",
		"/usr/bin/bash":                          "sh",
		`C:\Git\usr\bin\bash.exe`:                "sh",
		"cmd":                                    "cmd",
		"cmd.exe":                                "cmd",
		"powershell":                             "powershell",
		"PowerShell":                             "powershell",
		"pwsh":                                   "powershell",
		"ps":                                     "powershell",
		`C:\Program Files\PowerShell\7\pwsh.exe`: "powershell",
		"fish":                                   "",
		"":                                       "",
	}
	for in, want := range cases {
		if got := normalizeShell(in); got != want {
			t.Errorf("normalizeShell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetectShell(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	noParent := func() string { return "" }

	t.Run("unix uses SHELL", func(t *testing.T) {
		if got := detectShell("linux", env(map[string]string{"SHELL": "/bin/zsh"}), noParent); got != "sh" {
			t.Errorf("got %q, want sh", got)
		}
	})
	t.Run("windows msystem means sh", func(t *testing.T) {
		if got := detectShell("windows", env(map[string]string{"MSYSTEM": "MINGW64"}), noParent); got != "sh" {
			t.Errorf("got %q, want sh", got)
		}
	})
	t.Run("windows parent beats inherited env", func(t *testing.T) {
		got := detectShell("windows", env(map[string]string{"MSYSTEM": "MINGW64"}), func() string { return "powershell.exe" })
		if got != "powershell" {
			t.Errorf("got %q, want powershell", got)
		}
	})
}
