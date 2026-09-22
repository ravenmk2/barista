package nodecli

import (
	"io"
	"testing"
)

func TestInstallCmdMirrorFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"22.14.0", "--mirror", "bogus"},
		{"22.14.0", "--mirror", "http://insecure.example.com/node"},
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

func TestInstallCmdVersionValidation(t *testing.T) {
	for _, args := range [][]string{
		{"node-22"},
		{"22.x"},
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
