package comp

import (
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/workspace"
	"barista/schemas"
)

// Completion candidates must never fail loudly or touch the network:
// any load error yields no candidates.

func jdkRegistry() *jdk.Registry {
	p, err := jdk.RegistryPath()
	if err != nil {
		return nil
	}
	reg, e := jdk.Load(p)
	if e != nil {
		return nil
	}
	return reg
}

func mavenRegistry() *maven.Registry {
	p, err := maven.RegistryPath()
	if err != nil {
		return nil
	}
	reg, e := maven.Load(p)
	if e != nil {
		return nil
	}
	return reg
}

func JdkNames() []string {
	reg := jdkRegistry()
	if reg == nil {
		return nil
	}
	out := make([]string, 0, len(reg.JDKs))
	for _, e := range reg.JDKs {
		out = append(out, e.Name+"\t"+e.Version)
	}
	return out
}

func JdkMajors() []string {
	reg := jdkRegistry()
	if reg == nil {
		return nil
	}
	seen := map[int]bool{}
	var out []string
	for _, e := range reg.JDKs {
		if seen[e.Major] {
			continue
		}
		seen[e.Major] = true
		out = append(out, strconv.Itoa(e.Major)+"\t"+e.Name+" "+e.Version)
	}
	return out
}

func JdkSpecs() []string {
	return append(JdkNames(), JdkMajors()...)
}

func MavenNames() []string {
	reg := mavenRegistry()
	if reg == nil {
		return nil
	}
	out := make([]string, 0, len(reg.Installations))
	for _, e := range reg.Installations {
		out = append(out, e.Name+"\t"+e.Version)
	}
	return out
}

func MavenSpecs() []string {
	reg := mavenRegistry()
	if reg == nil {
		return nil
	}
	out := make([]string, 0, 2*len(reg.Installations))
	for _, e := range reg.Installations {
		out = append(out, e.Name+"\t"+e.Version, e.Version+"\t"+e.Name)
	}
	return out
}

func workspaceRepos() *workspace.Workspace {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	if workspace.FindWorkspaceRoot(cwd) == "" {
		return nil
	}
	ws, err := workspace.Load(cwd)
	if err != nil {
		return nil
	}
	return ws
}

func RepoNames() []string {
	ws := workspaceRepos()
	if ws == nil {
		return nil
	}
	out := make([]string, 0, len(ws.Repos.Repos))
	for _, r := range ws.Repos.Repos {
		out = append(out, r.Name+"\t"+r.Path)
	}
	return out
}

func Labels() []string {
	ws := workspaceRepos()
	if ws == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range ws.Repos.Repos {
		for _, l := range r.Labels {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

func TargetOSes() []string {
	return []string{"linux", "darwin", "windows"}
}

func TargetArches() []string {
	return []string{"amd64", "arm64"}
}

func Shells() []string {
	return []string{"sh\tbash/zsh compatible", "cmd\tWindows cmd.exe", "powershell\tPowerShell / pwsh", "pwsh\talias of powershell", "ps\talias of powershell"}
}

func SchemaNames() []string {
	var out []string
	for _, e := range schemas.List() {
		out = append(out, e.Name+"\t"+e.Description)
	}
	return out
}

func Fn(candidates func() []string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return candidates(), cobra.ShellCompDirectiveNoFileComp
	}
}

// Dirs lets the shell complete directories only (used for path args/flags).
func Dirs(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}
