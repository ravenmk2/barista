package mvncli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

const classworldsLauncher = "org.codehaus.plexus.classworlds.launcher.Launcher"

type launchSpec struct {
	bin    string
	args   []string
	viaCmd bool
}

func resolveStartup(in planInput, repo *workspace.Repo) (string, string, *output.ErrInfo) {
	spec, src := in.startupFlag, "flag"
	if spec == "" && repo != nil {
		if v, ok := repo.Property("maven.startup"); ok && v != "" {
			spec, src = v, "repo"
		}
	}
	if spec == "" {
		if v, ok := in.wsCfg.Property("maven.startup"); ok && v != "" {
			spec, src = v, "workspace"
		}
	}
	if spec == "" {
		return "script", "default", nil
	}
	if spec != "jar" && spec != "script" {
		return "", "", &output.ErrInfo{
			Code:    output.CodeConfigError,
			Message: fmt.Sprintf("invalid maven.startup %q (from %s): want jar|script", spec, src),
		}
	}
	return spec, src, nil
}

func scriptLaunch(p *execPlan) launchSpec {
	return launchSpec{bin: p.mavenBin, args: p.args, viaCmd: p.goos == "windows"}
}

func jarLaunch(in planInput, p *execPlan) (launchSpec, string, *output.ErrInfo) {
	jars, err := filepath.Glob(filepath.Join(p.mavenHome, "boot", "plexus-classworlds-*.jar"))
	if err == nil && len(jars) != 1 {
		err = fmt.Errorf("want exactly 1 plexus-classworlds jar, found %d", len(jars))
	}
	if err != nil {
		return launchSpec{}, "", &output.ErrInfo{
			Code:    output.CodeMavenExecFailed,
			Message: fmt.Sprintf("%s: %v", filepath.ToSlash(p.mavenHome), err),
			Hint:    "fall back to the wrapper script with: --startup script",
		}
	}
	m2conf := filepath.Join(p.mavenHome, "bin", "m2.conf")
	if !fileExists(m2conf) {
		return launchSpec{}, "", &output.ErrInfo{
			Code:    output.CodeMavenExecFailed,
			Message: fmt.Sprintf("%s: no bin/m2.conf found", filepath.ToSlash(p.mavenHome)),
			Hint:    "fall back to the wrapper script with: --startup script",
		}
	}

	javaBin := "java"
	if p.javaHome != "" {
		javaBin = filepath.Join(p.javaHome, "bin", javaExeName(in.goos))
	}
	basedir := findBasedir(in.cwd, p.args)

	var args []string
	args = append(args, jvmConfigTokens(basedir)...)
	args = append(args, splitEnv("MAVEN_OPTS")...)
	args = append(args, splitEnv("MAVEN_DEBUG_OPTS")...)
	args = append(args, "-classpath", jars[0])
	args = append(args, "-Dclassworlds.conf="+m2conf)
	args = append(args, "-Dmaven.home="+p.mavenHome)
	jansi := filepath.Join(p.mavenHome, "lib", "jansi-native")
	if dirExists(jansi) {
		args = append(args, "-Dlibrary.jansi.path="+jansi)
	}
	args = append(args, "-Dmaven.multiModuleProjectDirectory="+basedir)
	args = append(args, classworldsLauncher)
	if supportsMavenArgs(p.mavenVersion) {
		args = append(args, splitEnv("MAVEN_ARGS")...)
	}
	args = append(args, p.args...)
	return launchSpec{bin: javaBin, args: args}, basedir, nil
}

func javaExeName(goos string) string {
	if goos == "windows" {
		return "java.exe"
	}
	return "java"
}

func findBasedir(cwd string, args []string) string {
	if v := os.Getenv("MAVEN_BASEDIR"); v != "" {
		return v
	}
	dir := cwd
	for i, a := range args {
		if a != "-f" && a != "--file" || i+1 >= len(args) {
			continue
		}
		f := args[i+1]
		if !filepath.IsAbs(f) {
			f = filepath.Join(cwd, f)
		}
		if st, err := os.Stat(f); err == nil {
			if st.IsDir() {
				dir = f
			} else {
				dir = filepath.Dir(f)
			}
		}
		break
	}
	d := dir
	for {
		if dirExists(filepath.Join(d, ".mvn")) {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return cwd
		}
		d = parent
	}
}

func jvmConfigTokens(basedir string) []string {
	data, err := os.ReadFile(filepath.Join(basedir, ".mvn", "jvm.config"))
	if err != nil {
		return nil
	}
	var tokens []string
	for _, line := range strings.Split(string(data), "\n") {
		tokens = append(tokens, strings.Fields(line)...)
	}
	return tokens
}

func splitEnv(key string) []string {
	return strings.Fields(os.Getenv(key))
}

func supportsMavenArgs(version string) bool {
	segs, _, err := maven.ParseVersion(version)
	if err != nil || len(segs) == 0 {
		return false
	}
	if segs[0] != 3 {
		return segs[0] > 3
	}
	return len(segs) > 1 && segs[1] >= 9
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
