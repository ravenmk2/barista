package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"barista/internal/gitrun"
	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
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
			fmt.Sprintf("%s exists but is not a git work tree", filepath.ToSlash(dir)), "")
	}
	origin, err := gitrun.OriginURL(ctx, dir)
	if err != nil {
		return failed(repo.Name, "workspace", check, output.CodeGitError,
			fmt.Sprintf("cannot read origin of %s: %v", filepath.ToSlash(dir), err), "")
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

func checkSettingsFile(wsRoot, file, check, injected string) output.Result {
	p := filepath.Join(wsRoot, ".barista", "maven", file)
	if _, err := os.Stat(p); err != nil {
		return skipped(file, "workspace", check, "not present (optional)")
	}
	return ok(file, "workspace", check, map[string]any{"injected": injected})
}
