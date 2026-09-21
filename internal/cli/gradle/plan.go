package gradlecli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

type planInput struct {
	cwd               string
	goos              string
	jdkFlag           string
	passthrough       []string
	wsRoot            string
	repos             []workspace.Repo
	props             workspace.Properties
	gradleReg         *gradle.Registry
	jdkReg            *jdk.Registry
	javaVersionMajor  int
	javaVersionDistro string
	javaVersionFile   string
	wrapperVersion    string
	wrapperFile       string
}

type launchSpec struct {
	bin    string
	args   []string
	viaCmd bool
}

type execPlan struct {
	goos           string
	wsRoot         string
	repoName       string
	repoPath       string
	gradleName     string
	gradleVersion  string
	gradleHome     string
	gradleBin      string
	gradleSrc      string
	gradleFile     string
	javaHome       string
	jdkSpec        string
	jdkSrc         string
	jdkFile        string
	jdkName        string
	jdkVersion     string
	initScripts    []string
	gradleUserHome string
	passthrough    []string
	args           []string
	launch         launchSpec
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

	gradleEntry, gradleSrc, e := resolveGradle(in)
	if e != nil {
		return nil, e
	}
	p.gradleName = gradleEntry.Name
	p.gradleVersion = gradleEntry.Version
	p.gradleHome = gradleEntry.Path
	p.gradleSrc = gradleSrc
	if gradleSrc == "wrapper" {
		p.gradleFile = in.wrapperFile
	}
	bin := "bin/gradle"
	if in.goos == "windows" {
		bin = "bin/gradle.bat"
	}
	p.gradleBin = filepath.Join(gradleEntry.Path, filepath.FromSlash(bin))

	jdkEntry, spec, jdkSrc, e := resolveJdk(in, repo)
	if e != nil {
		return nil, e
	}
	if jdkEntry != nil {
		p.javaHome = jdkEntry.Path
		p.jdkSpec = spec
		p.jdkSrc = jdkSrc
		if jdkSrc == "java-version" {
			p.jdkFile = in.javaVersionFile
		}
		p.jdkName = jdkEntry.Name
		p.jdkVersion = jdkEntry.Version
	}

	if in.wsRoot != "" && !hasFlagArg(in.passthrough, "-I", "--init-script") {
		for _, name := range []string{"init.gradle", "init.gradle.kts"} {
			s := filepath.Join(in.wsRoot, ".barista", "gradle", name)
			if fileExists(s) {
				p.initScripts = append(p.initScripts, s)
			}
		}
	}

	if in.wsRoot != "" && !hasFlagArg(in.passthrough, "-g", "--gradle-user-home") && !hasSystemProp(in.passthrough, "gradle.user.home") {
		if v, ok := in.props.String("gradle.user.home"); ok && v != "" {
			v = workspace.ExpandHome(v)
			if !filepath.IsAbs(v) {
				v = filepath.Join(in.wsRoot, v)
			}
			p.gradleUserHome = v
		}
	}

	p.args = buildArgs(p)
	p.launch = launchSpec{bin: p.gradleBin, args: p.args, viaCmd: p.goos == "windows"}
	return p, nil
}

func resolveGradle(in planInput) (*gradle.Entry, string, *output.ErrInfo) {
	if in.wrapperVersion != "" {
		entry, _, e := in.gradleReg.Resolve(in.wrapperVersion)
		if e != nil {
			return nil, "", &output.ErrInfo{
				Code:    output.CodeGradleNotFound,
				Message: fmt.Sprintf("gradle %s (from gradle-wrapper.properties) is not registered", in.wrapperVersion),
				Hint:    "run: barista gradle install " + in.wrapperVersion,
			}
		}
		return entry, "wrapper", nil
	}
	if v, ok := in.props.String("gradle.default"); ok && v != "" {
		if e := in.gradleReg.Find(v); e != nil {
			return e, "workspace", nil
		}
		return nil, "", &output.ErrInfo{
			Code:    output.CodeGradleNotFound,
			Message: fmt.Sprintf("default gradle %q (from workspace config) is not registered", v),
			Hint:    "run: barista gradle list; fix with: barista gradle set-default <name> --scope workspace",
		}
	}
	if in.gradleReg.Default != "" {
		if e := in.gradleReg.Find(in.gradleReg.Default); e != nil {
			return e, "user", nil
		}
		return nil, "", &output.ErrInfo{
			Code:    output.CodeGradleNotFound,
			Message: fmt.Sprintf("default gradle %q is not registered", in.gradleReg.Default),
			Hint:    "run: barista gradle list; fix with: barista gradle set-default <name>",
		}
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeGradleNotFound,
		Message: "no default gradle configured",
		Hint:    "set one with: barista gradle set-default <name>",
	}
}

func resolveJdk(in planInput, repo *workspace.Repo) (*jdk.Entry, string, string, *output.ErrInfo) {
	spec, src := in.jdkFlag, "flag"
	if spec == "" && in.javaVersionMajor > 0 {
		entry, vSpec := jdk.ResolveJavaVersionSpec(in.jdkReg, in.javaVersionMajor, in.javaVersionDistro)
		if entry == nil {
			return nil, "", "", &output.ErrInfo{
				Code:    output.CodeJDKNotFound,
				Message: fmt.Sprintf("jdk %q (from .java-version) is not registered", vSpec),
				Hint:    "run: barista jdk list",
			}
		}
		return entry, vSpec, "java-version", nil
	}
	if spec == "" && repo != nil {
		if v, ok := repo.Property("jdk"); ok && v != "" {
			spec, src = v, "repo"
		}
	}
	if spec == "" {
		if v, ok := in.props.String("jdk"); ok && v != "" {
			spec, src = v, "workspace"
		}
	}
	if spec == "" && in.gradleReg.Jdk != "" {
		spec, src = in.gradleReg.Jdk, "user"
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
	for _, s := range p.initScripts {
		args = append(args, "-I", s)
	}
	if p.gradleUserHome != "" {
		args = append(args, "--gradle-user-home", p.gradleUserHome)
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
