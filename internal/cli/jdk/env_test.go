package jdkcli

import (
	"strings"
	"testing"

	"barista/internal/output"
)

func TestRenderEnvSh(t *testing.T) {
	got, e := renderEnv("sh", `/opt/jdk/temurin 17`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := "export JAVA_HOME='/opt/jdk/temurin 17'\n" +
		`export PATH="$JAVA_HOME/bin:$PATH"` + "\n"
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
	got, e := renderEnv("cmd", `C:\Program Files\Java\jdk-17`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := `set "JAVA_HOME=C:\Program Files\Java\jdk-17"` + "\r\n" +
		`set "PATH=%JAVA_HOME%\bin;%PATH%"` + "\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEnvPowerShell(t *testing.T) {
	got, e := renderEnv("powershell", `C:\jdk's\jdk-17`)
	if e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	want := "$env:JAVA_HOME = 'C:\\jdk''s\\jdk-17'\n" +
		`$env:PATH = "$env:JAVA_HOME\bin;$env:PATH"` + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderEnvUnsupportedShell(t *testing.T) {
	_, e := renderEnv("fish", "/opt/jdk")
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
	t.Run("unix unknown shell undetected", func(t *testing.T) {
		if got := detectShell("darwin", env(map[string]string{"SHELL": "/bin/fish"}), noParent); got != "" {
			t.Errorf("got %q, want undetected", got)
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
	t.Run("windows SHELL means sh", func(t *testing.T) {
		if got := detectShell("windows", env(map[string]string{"SHELL": "/usr/bin/bash"}), noParent); got != "sh" {
			t.Errorf("got %q, want sh", got)
		}
	})
	t.Run("windows parent powershell", func(t *testing.T) {
		got := detectShell("windows", env(map[string]string{}), func() string { return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` })
		if got != "powershell" {
			t.Errorf("got %q, want powershell", got)
		}
	})
	t.Run("windows parent cmd", func(t *testing.T) {
		got := detectShell("windows", env(map[string]string{}), func() string { return `C:\Windows\System32\cmd.exe` })
		if got != "cmd" {
			t.Errorf("got %q, want cmd", got)
		}
	})
	t.Run("windows undetected", func(t *testing.T) {
		if got := detectShell("windows", env(map[string]string{}), noParent); got != "" {
			t.Errorf("got %q, want undetected", got)
		}
	})
}
