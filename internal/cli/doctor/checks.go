package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"barista/internal/deps"
	"barista/internal/gitrun"
	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/uv"
	"barista/internal/workspace"
)

func ok(name, scope, check string, detail map[string]any) output.Result {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["scope"] = scope
	detail["check"] = check
	return output.Result{Name: name, Status: output.StatusOK, Action: "check", Detail: detail}
}

func failed(name, scope, check, code, message, hint string) output.Result {
	return output.Result{
		Name:   name,
		Status: output.StatusFailed,
		Action: "check",
		Detail: map[string]any{"scope": scope, "check": check},
		Error:  &output.ErrInfo{Code: code, Message: message, Hint: hint},
	}
}

func skipped(name, scope, check, reason string) output.Result {
	return output.Result{
		Name:   name,
		Status: output.StatusSkipped,
		Action: "check",
		Detail: map[string]any{"scope": scope, "check": check, "reason": reason},
	}
}

func errResult(name, scope, check string, e *output.ErrInfo) output.Result {
	return output.Result{
		Name:   name,
		Status: output.StatusFailed,
		Action: "check",
		Detail: map[string]any{"scope": scope, "check": check},
		Error:  e,
	}
}

func checkGitOnPath() output.Result {
	const name, check = "git", "gitOnPath"
	if _, err := exec.LookPath("git"); err != nil {
		return failed(name, "user", check, output.CodeGitError, "git not found on PATH", "install git and make sure it is on PATH")
	}
	return ok(name, "user", check, nil)
}

func checkUv() output.Result {
	const name, check = "uv", "uvAvailable"
	if p, err := exec.LookPath("uv"); err == nil {
		return ok(name, "user", check, map[string]any{"path": filepath.ToSlash(p)})
	}
	if dir, err := uv.LocalBinDir(); err == nil {
		if _, err := os.Stat(filepath.Join(dir, uv.BinaryName())); err == nil {
			return skipped(name, "user", check, "installed at "+filepath.ToSlash(dir)+" but not on PATH")
		}
	}
	return skipped(name, "user", check, "not installed (run: barista uv install)")
}

func checkJavaHome() output.Result {
	const name, check = "JAVA_HOME", "javaHomeEnv"
	v := os.Getenv("JAVA_HOME")
	if v == "" {
		return skipped(name, "user", check, "not set (ambient; barista resolves JDKs from its own registry)")
	}
	javaBin := filepath.Join(v, "bin", exeName("java"))
	if _, err := os.Stat(javaBin); err != nil {
		return failed(name, "user", check, output.CodeNotAJDK,
			fmt.Sprintf("JAVA_HOME=%s has no bin/%s", filepath.ToSlash(v), exeName("java")),
			"fix JAVA_HOME or unset it")
	}
	return ok(name, "user", check, map[string]any{"javaHome": filepath.ToSlash(v)})
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func checkJdkEntry(e jdk.Entry) output.Result {
	const check = "jdkInstall"
	javaBin := filepath.Join(e.Path, "bin", exeName("java"))
	if _, err := os.Stat(javaBin); err != nil {
		return failed(e.Name, "user", check, output.CodeJDKNotFound,
			fmt.Sprintf("bin/%s not found at %s", exeName("java"), filepath.ToSlash(e.Path)),
			"run: barista jdk remove "+e.Name+"; or re-add with: barista jdk add "+e.Name+" <path>")
	}
	return ok(e.Name, "user", check, map[string]any{
		"version": e.Version, "major": e.Major, "path": filepath.ToSlash(e.Path),
	})
}

func checkJdkProbe(e jdk.Entry) output.Result {
	const check = "jdkProbe"
	info, perr := jdk.Probe(e.Path)
	if perr != nil {
		perr.Hint = "run: barista jdk remove " + e.Name + "; or re-add with: barista jdk add " + e.Name + " <path>"
		return errResult(e.Name, "user", check, perr)
	}
	if info.Major != e.Major || info.Version != e.Version {
		return failed(e.Name, "user", check, output.CodeJDKMajorMismatch,
			fmt.Sprintf("registered %s (major %d) but probe reports %s (major %d)", e.Version, e.Major, info.Version, info.Major),
			"re-register with: barista jdk remove "+e.Name+" && barista jdk add "+e.Name+" "+filepath.ToSlash(e.Path))
	}
	return ok(e.Name, "user", check, map[string]any{"version": info.Version, "distro": info.Distro})
}

func checkJdkDefaults(reg *jdk.Registry) output.Result {
	const name, check = "jdk.json defaults", "jdkDefaults"
	if len(reg.Defaults) == 0 {
		return skipped(name, "user", check, "no defaults configured")
	}
	var missing []string
	for major, n := range reg.Defaults {
		if reg.Find(n) == nil {
			missing = append(missing, fmt.Sprintf("%s→%s", major, n))
		}
	}
	if len(missing) > 0 {
		return failed(name, "user", check, output.CodeJDKNotFound,
			fmt.Sprintf("defaults point at unregistered JDKs: %v", missing),
			"fix with: barista jdk set-default <major> <name>")
	}
	return ok(name, "user", check, map[string]any{"defaults": len(reg.Defaults)})
}

func checkInstallDir(file, check, dir, resetHint string) output.Result {
	if dir == "" {
		return skipped(file, "user", check, "not set (built-in default)")
	}
	if _, err := os.Stat(dir); err != nil {
		return failed(file, "user", check, output.CodeConfigError,
			fmt.Sprintf("installDir %s does not exist", filepath.ToSlash(dir)),
			"create it or run: "+resetHint)
	}
	return ok(file, "user", check, map[string]any{"installDir": filepath.ToSlash(dir)})
}

func checkMavenEntry(e maven.Entry) output.Result {
	const check = "mavenInstall"
	info, perr := maven.Probe(e.Path)
	if perr != nil {
		perr.Hint = "run: barista maven remove " + e.Name + "; or re-add with: barista maven add <path> --name " + e.Name
		return errResult(e.Name, "user", check, perr)
	}
	if info.Version != e.Version {
		return failed(e.Name, "user", check, output.CodeMavenProbeFailed,
			fmt.Sprintf("registered %s but maven-core jar reports %s", e.Version, info.Version),
			"re-register with: barista maven remove "+e.Name+" && barista maven add "+filepath.ToSlash(e.Path))
	}
	return ok(e.Name, "user", check, map[string]any{"version": info.Version, "path": filepath.ToSlash(e.Path)})
}

func checkMavenDefault(reg *maven.Registry) output.Result {
	const name, check = "maven.json default", "mavenDefault"
	if reg.Default == "" {
		return skipped(name, "user", check, "no default configured")
	}
	if reg.Find(reg.Default) == nil {
		return failed(name, "user", check, output.CodeMavenNotFound,
			fmt.Sprintf("default maven %q is not registered", reg.Default),
			"fix with: barista maven set-default <name>")
	}
	return ok(name, "user", check, map[string]any{"default": reg.Default})
}

func checkMavenJdk(reg *maven.Registry, jdkReg *jdk.Registry) output.Result {
	const name, check = "maven.json jdk", "mavenJdk"
	if reg.Jdk == "" {
		return skipped(name, "user", check, "no jdk configured (ambient fallback)")
	}
	if _, _, e := jdkReg.Resolve(reg.Jdk); e != nil {
		return failed(name, "user", check, output.CodeJDKNotFound,
			fmt.Sprintf("maven jdk %q is not registered", reg.Jdk),
			"fix with: barista maven set-jdk <major|name>")
	}
	return ok(name, "user", check, map[string]any{"jdk": reg.Jdk})
}

func checkGradleEntry(e gradle.Entry) output.Result {
	const check = "gradleInstall"
	info, perr := gradle.Probe(e.Path)
	if perr != nil {
		perr.Hint = "run: barista gradle remove " + e.Name + "; or re-add with: barista gradle add <path> --name " + e.Name
		return errResult(e.Name, "user", check, perr)
	}
	if info.Version != gradle.ProbeVersion(e.Version) {
		return failed(e.Name, "user", check, output.CodeGradleProbeFailed,
			fmt.Sprintf("registered %s but probe reports %s", e.Version, info.Version),
			"re-register with: barista gradle remove "+e.Name+" && barista gradle add "+filepath.ToSlash(e.Path))
	}
	return ok(e.Name, "user", check, map[string]any{"version": info.Version, "path": filepath.ToSlash(e.Path)})
}

func checkGradleDefault(reg *gradle.Registry) output.Result {
	const name, check = "gradle.json default", "gradleDefault"
	if reg.Default == "" {
		return skipped(name, "user", check, "no default configured")
	}
	if reg.Find(reg.Default) == nil {
		return failed(name, "user", check, output.CodeGradleNotFound,
			fmt.Sprintf("default gradle %q is not registered", reg.Default),
			"fix with: barista gradle set-default <name>")
	}
	return ok(name, "user", check, map[string]any{"default": reg.Default})
}

func checkGradleJdk(reg *gradle.Registry, jdkReg *jdk.Registry) output.Result {
	const name, check = "gradle.json jdk", "gradleJdk"
	if reg.Jdk == "" {
		return skipped(name, "user", check, "no jdk configured (ambient fallback)")
	}
	if _, _, e := jdkReg.Resolve(reg.Jdk); e != nil {
		return failed(name, "user", check, output.CodeJDKNotFound,
			fmt.Sprintf("gradle jdk %q is not registered", reg.Jdk),
			"fix with: barista gradle set-jdk <major|name>")
	}
	return ok(name, "user", check, map[string]any{"jdk": reg.Jdk})
}

func checkJdkSpec(scope, subject, spec, howSet string, jdkReg *jdk.Registry) output.Result {
	const check = "jdkSpec"
	if _, _, e := jdkReg.Resolve(spec); e != nil {
		return failed(subject, scope, check, output.CodeJDKNotFound,
			fmt.Sprintf("jdk %q (from %s) is not registered", spec, howSet),
			"run: barista jdk list; fix the property or register the JDK")
	}
	return ok(subject, scope, check, map[string]any{"jdk": spec, "via": howSet})
}

func checkMavenDefaultSpec(scope, subject, spec, howSet string, mavenReg *maven.Registry) output.Result {
	const check = "mavenDefaultSpec"
	if mavenReg.Find(spec) == nil {
		return failed(subject, scope, check, output.CodeMavenNotFound,
			fmt.Sprintf("maven %q (from %s) is not registered", spec, howSet),
			"run: barista maven list; fix the property or register the installation")
	}
	return ok(subject, scope, check, map[string]any{"maven": spec, "via": howSet})
}

func checkGradleDefaultSpec(scope, subject, spec, howSet string, gradleReg *gradle.Registry) output.Result {
	const check = "gradleDefaultSpec"
	if gradleReg.Find(spec) == nil {
		return failed(subject, scope, check, output.CodeGradleNotFound,
			fmt.Sprintf("gradle %q (from %s) is not registered", spec, howSet),
			"run: barista gradle list; fix the property or register the installation")
	}
	return ok(subject, scope, check, map[string]any{"gradle": spec, "via": howSet})
}

func checkRepoCheckout(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo, deep bool) output.Result {
	const check = "repoCheckout"
	dir := ws.AbsPath(repo)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return skipped(repo.Name, "workspace", check, "not cloned (run: barista git clone --repo "+repo.Name+")")
	}
	if !deep {
		return ok(repo.Name, "workspace", check, map[string]any{"path": repo.Path})
	}
	if !gitrun.IsGitRepo(ctx, dir) {
		return failed(repo.Name, "workspace", check, output.CodeGitError,
			fmt.Sprintf("%s exists but is not a git work tree", filepath.ToSlash(dir)),
			"remove the directory and re-clone with: barista git clone --repo "+repo.Name)
	}
	origin, err := gitrun.OriginURL(ctx, dir)
	if err != nil {
		return failed(repo.Name, "workspace", check, output.CodeGitError,
			fmt.Sprintf("cannot read origin of %s: %v", filepath.ToSlash(dir), err),
			"inspect the remote with: git -C "+filepath.ToSlash(dir)+" remote -v")
	}
	if workspace.NormalizeURL(origin) != workspace.NormalizeURL(repo.ResolvedURL) {
		return failed(repo.Name, "workspace", check, output.CodeRepoRemoteMismatch,
			fmt.Sprintf("origin %q does not match manifest url %q", origin, repo.ResolvedURL),
			"fix the url in .barista/repos.json or re-clone")
	}
	return ok(repo.Name, "workspace", check, map[string]any{"path": repo.Path, "origin": origin})
}

func checkMavenLaunch(scope, subject, value, howSet string) output.Result {
	const check = "mavenLaunch"
	if value != "java" && value != "script" {
		return failed(subject, scope, check, output.CodeConfigError,
			fmt.Sprintf("maven.launch %q (from %s) is invalid", value, howSet),
			"valid values: java, script")
	}
	return ok(subject, scope, check, map[string]any{"launch": value, "via": howSet})
}

func checkRepoDeps(ws *workspace.Workspace) output.Result {
	const name, check = "repos.json deps", "repoDeps"
	g := deps.Build(ws.Repos.Repos)
	if !g.HasDangling() {
		total := 0
		for _, r := range ws.Repos.Repos {
			total += len(g.Deps(r.Name))
		}
		if total == 0 {
			return skipped(name, "workspace", check, "no deps declared")
		}
		return ok(name, "workspace", check, map[string]any{"edges": total})
	}
	var parts []string
	for _, r := range ws.Repos.Repos {
		for _, d := range g.Dangling(r.Name) {
			parts = append(parts, r.Name+"→"+d)
		}
	}
	return failed(name, "workspace", check, output.CodeRepoNotFound,
		fmt.Sprintf("deps reference unknown repos: %v", parts),
		"fix the deps entries in .barista/repos.json")
}

func checkWorkspaceFile(wsRoot, domain, file, check, injected string) output.Result {
	p := filepath.Join(wsRoot, ".barista", domain, file)
	if _, err := os.Stat(p); err != nil {
		return skipped(file, "workspace", check, "not present (optional)")
	}
	return ok(file, "workspace", check, map[string]any{"injected": injected})
}

func checkJavaVersionFile(ws *workspace.Workspace, repo workspace.Repo, jdkReg *jdk.Registry) output.Result {
	const check = "javaVersionFile"
	p := filepath.Join(ws.AbsPath(repo), ".java-version")
	data, err := os.ReadFile(p)
	if err != nil {
		return skipped(repo.Name, "workspace", check, "no .java-version")
	}
	major, distro, parsed := jdk.ParseJavaVersionFile(string(data))
	if !parsed {
		return failed(repo.Name, "workspace", check, output.CodeConfigError,
			fmt.Sprintf("%s is not parseable", filepath.ToSlash(p)),
			"write a version like 17 or temurin-17.0.13 into .java-version")
	}
	entry, spec := jdk.ResolveJavaVersionSpec(jdkReg, major, distro)
	if entry == nil {
		return failed(repo.Name, "workspace", check, output.CodeJDKNotFound,
			fmt.Sprintf("jdk %q (from %s) is not registered", spec, filepath.ToSlash(p)),
			"run: barista jdk list; register a matching JDK")
	}
	return ok(repo.Name, "workspace", check, map[string]any{
		"file": filepath.ToSlash(p), "jdk": entry.Name, "version": entry.Version,
	})
}

func checkMavenWrapperFile(ws *workspace.Workspace, repo workspace.Repo, mavenReg *maven.Registry) output.Result {
	const check = "mavenWrapperFile"
	p := filepath.Join(ws.AbsPath(repo), ".mvn", "wrapper", "maven-wrapper.properties")
	data, err := os.ReadFile(p)
	if err != nil {
		return skipped(repo.Name, "workspace", check, "no maven-wrapper.properties")
	}
	version, parsed := maven.ExtractWrapperVersion(string(data))
	if !parsed {
		return failed(repo.Name, "workspace", check, output.CodeConfigError,
			fmt.Sprintf("%s has no recognizable distributionUrl", filepath.ToSlash(p)),
			"fix the distributionUrl in maven-wrapper.properties")
	}
	if _, _, e := mavenReg.Resolve(version); e != nil {
		return failed(repo.Name, "workspace", check, output.CodeMavenNotFound,
			fmt.Sprintf("maven %s (from %s) is not registered", version, filepath.ToSlash(p)),
			"run: barista maven install "+version)
	}
	return ok(repo.Name, "workspace", check, map[string]any{
		"file": filepath.ToSlash(p), "maven": version,
	})
}

func checkGradleWrapperFile(ws *workspace.Workspace, repo workspace.Repo, gradleReg *gradle.Registry) output.Result {
	const check = "gradleWrapperFile"
	p := filepath.Join(ws.AbsPath(repo), "gradle", "wrapper", "gradle-wrapper.properties")
	data, err := os.ReadFile(p)
	if err != nil {
		return skipped(repo.Name, "workspace", check, "no gradle-wrapper.properties")
	}
	version, parsed := gradle.ExtractWrapperVersion(string(data))
	if !parsed {
		return failed(repo.Name, "workspace", check, output.CodeConfigError,
			fmt.Sprintf("%s has no recognizable distributionUrl", filepath.ToSlash(p)),
			"fix the distributionUrl in gradle-wrapper.properties")
	}
	if _, _, e := gradleReg.Resolve(version); e != nil {
		return failed(repo.Name, "workspace", check, output.CodeGradleNotFound,
			fmt.Sprintf("gradle %s (from %s) is not registered", version, filepath.ToSlash(p)),
			"run: barista gradle install "+version)
	}
	return ok(repo.Name, "workspace", check, map[string]any{
		"file": filepath.ToSlash(p), "gradle": version,
	})
}
