package gradlecli

import (
	"io"
	"testing"
)

func TestInstallCmdMirrorFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"8.10.2", "--mirror", "bogus"},
		{"8.10.2", "--mirror", "http://insecure.example.com/gradle"},
	} {
		cmd := installCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("args %v: want usage error", args)
		}
	}
}
