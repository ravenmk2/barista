package javacli

import (
	"fmt"
	"path/filepath"

	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

type planInput struct {
	cwd         string
	goos        string
	jdkFlag     string
	passthrough []string
	ambientBin  string
	wsRoot      string
	repos       []workspace.Repo
	props       workspace.Properties
	jdkReg      *jdk.Registry
}

type execPlan struct {
	goos        string
	wsRoot      string
	repoName    string
	repoPath    string
	javaHome    string
	jdkSpec     string
	jdkSrc      string
	jdkName     string
	jdkVersion  string
	javaBin     string
	passthrough []string
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

	spec, src := in.jdkFlag, "flag"
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

	if spec == "" {
		if in.ambientBin == "" {
			return nil, &output.ErrInfo{
				Code:    output.CodeJDKNotFound,
				Message: "no jdk configured and java not found on PATH",
				Hint:    "run: barista jdk list; or pass --jdk <major|name>",
			}
		}
		p.jdkSrc = "ambient"
		p.javaBin = in.ambientBin
		return p, nil
	}

	entry, _, e := in.jdkReg.Resolve(spec)
	if e != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeJDKNotFound,
			Message: fmt.Sprintf("jdk %q (from %s) is not registered", spec, src),
			Hint:    "run: barista jdk list",
		}
	}
	p.javaHome = entry.Path
	p.jdkSpec = spec
	p.jdkSrc = src
	p.jdkName = entry.Name
	p.jdkVersion = entry.Version
	bin := "bin/java"
	if in.goos == "windows" {
		bin = "bin/java.exe"
	}
	p.javaBin = filepath.Join(entry.Path, filepath.FromSlash(bin))
	return p, nil
}
