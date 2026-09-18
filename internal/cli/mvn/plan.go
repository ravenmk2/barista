package mvncli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

type planInput struct {
	cwd         string
	goos        string
	jdkFlag     string
	passthrough []string
	wsRoot      string
	repos       []workspace.Repo
	wsCfg       workspace.ConfigFile
	mavenReg    *maven.Registry
	jdkReg      *jdk.Registry
}

type execPlan struct {
	goos             string
	wsRoot           string
	repoName         string
	repoPath         string
	mavenName        string
	mavenVersion     string
	mavenHome        string
	mavenBin         string
	mavenSrc         string
	javaHome         string
	jdkSpec          string
	jdkSrc           string
	jdkName          string
	jdkVersion       string
	settings         string
	settingsSecurity string
	repoLocal        string
	passthrough      []string
	args             []string
}

func planExec(in planInput) (*execPlan, *output.ErrInfo) {
	p := &execPlan{goos: in.goos, wsRoot: in.wsRoot, passthrough: in.passthrough}

	var repo *workspace.Repo
	if in.wsRoot != "" {
		ws := &workspace.Workspace{Root: in.wsRoot, Repos: &workspace.ReposFile{Repos: in.repos}}
		repo = ws.MatchRepo(in.cwd)
	}
	if repo != nil {
		p.repoName = repo.Name
		p.repoPath = repo.Path
	}

	mvnEntry, mvnSrc, e := resolveMaven(in)
	if e != nil {
		return nil, e
	}
	p.mavenName = mvnEntry.Name
	p.mavenVersion = mvnEntry.Version
	p.mavenHome = mvnEntry.Path
	p.mavenSrc = mvnSrc
	bin := "bin/mvn"
	if in.goos == "windows" {
		bin = "bin/mvn.cmd"
	}
	p.mavenBin = filepath.Join(mvnEntry.Path, filepath.FromSlash(bin))

	jdkEntry, spec, jdkSrc, e := resolveJdk(in, repo)
	if e != nil {
		return nil, e
	}
	if jdkEntry != nil {
		p.javaHome = jdkEntry.Path
		p.jdkSpec = spec
		p.jdkSrc = jdkSrc
		p.jdkName = jdkEntry.Name
		p.jdkVersion = jdkEntry.Version
	}

	if in.wsRoot != "" && !hasFlagArg(in.passthrough, "-s", "--settings") {
		s := filepath.Join(in.wsRoot, ".barista", "maven", "settings.xml")
		if fileExists(s) {
			p.settings = s
		}
		sec := filepath.Join(in.wsRoot, ".barista", "maven", "settings-security.xml")
		if fileExists(sec) && !hasSystemProp(in.passthrough, "settings.security") {
			p.settingsSecurity = sec
		}
	}

	if in.wsRoot != "" && !hasSystemProp(in.passthrough, "maven.repo.local") {
		if v, ok := in.wsCfg.Property("maven.repo.local"); ok && v != "" {
			v = workspace.ExpandHome(v)
			if !filepath.IsAbs(v) {
				v = filepath.Join(in.wsRoot, v)
			}
			p.repoLocal = v
		}
	}

	p.args = buildArgs(p)
	return p, nil
}

func resolveMaven(in planInput) (*maven.Entry, string, *output.ErrInfo) {
	if v, ok := in.wsCfg.Property("maven.default"); ok && v != "" {
		if e := in.mavenReg.Find(v); e != nil {
			return e, "workspace", nil
		}
		return nil, "", &output.ErrInfo{
			Code:    output.CodeMavenNotFound,
			Message: fmt.Sprintf("default maven %q (from workspace config) is not registered", v),
			Hint:    "run: barista maven list; fix with: barista maven set-default <name> --scope workspace",
		}
	}
	if in.mavenReg.Default != "" {
		if e := in.mavenReg.Find(in.mavenReg.Default); e != nil {
			return e, "user", nil
		}
		return nil, "", &output.ErrInfo{
			Code:    output.CodeMavenNotFound,
			Message: fmt.Sprintf("default maven %q is not registered", in.mavenReg.Default),
			Hint:    "run: barista maven list; fix with: barista maven set-default <name>",
		}
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeMavenNotFound,
		Message: "no default maven configured",
		Hint:    "set one with: barista maven set-default <name>",
	}
}

func resolveJdk(in planInput, repo *workspace.Repo) (*jdk.Entry, string, string, *output.ErrInfo) {
	spec, src := in.jdkFlag, "flag"
	if spec == "" && repo != nil {
		if v, ok := repo.Property("maven.jdk"); ok && v != "" {
			spec, src = v, "repo"
		}
	}
	if spec == "" {
		if v, ok := in.wsCfg.Property("maven.jdk"); ok && v != "" {
			spec, src = v, "workspace"
		}
	}
	if spec == "" && in.mavenReg.Jdk != "" {
		spec, src = in.mavenReg.Jdk, "user"
	}
	if spec == "" {
		return nil, "", "", nil
	}
	entry, _, e := in.jdkReg.Resolve(spec)
	if e != nil {
		return nil, "", "", &output.ErrInfo{
			Code:    output.CodeJDKNotFound,
			Message: fmt.Sprintf("jdk %q (from %s) is not registered", spec, src),
			Hint:    "run: barista jdk list",
		}
	}
	return entry, spec, src, nil
}

func buildArgs(p *execPlan) []string {
	var args []string
	if p.settings != "" {
		args = append(args, "-s", p.settings)
	}
	if p.settingsSecurity != "" {
		args = append(args, "-Dsettings.security="+p.settingsSecurity)
	}
	if p.repoLocal != "" {
		args = append(args, "-Dmaven.repo.local="+p.repoLocal)
	}
	return append(args, p.passthrough...)
}

func hasFlagArg(args []string, forms ...string) bool {
	for _, a := range args {
		for _, f := range forms {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

func hasSystemProp(args []string, key string) bool {
	prefix := "-D" + key
	for _, a := range args {
		if a == prefix || strings.HasPrefix(a, prefix+"=") {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
