package nodecli

import (
	"fmt"
	"path/filepath"
	"strings"

	"barista/internal/node"
	"barista/internal/output"
	"barista/internal/workspace"
)

type planInput struct {
	cwd         string
	goos        string
	tool        string
	nodeFlag    string
	passthrough []string
	ambientBin  string
	wsRoot      string
	repos       []workspace.Repo
	props       workspace.Properties
	nodeReg     *node.Registry
	fileVersion string
	filePath    string
}

type execPlan struct {
	goos        string
	tool        string
	wsRoot      string
	repoName    string
	repoPath    string
	nodeSpec    string
	nodeName    string
	nodeVersion string
	nodeHome    string
	nodeSrc     string
	nodeFile    string
	pathEntry   string
	bin         string
	viaCmd      bool
	passthrough []string
}

func planExec(in planInput) (*execPlan, *output.ErrInfo) {
	p := &execPlan{goos: in.goos, tool: in.tool, wsRoot: in.wsRoot, passthrough: in.passthrough}

	var repo *workspace.Repo
	if in.wsRoot != "" {
		ws := &workspace.Workspace{Root: in.wsRoot, Repos: &workspace.ReposFile{Repos: in.repos}}
		repo = ws.MatchRepo(in.cwd)
	}
	if repo != nil {
		p.repoName = repo.Name
		p.repoPath = repo.Path
	}

	spec, src := in.nodeFlag, "flag"
	if spec == "" && in.fileVersion != "" {
		spec, src = in.fileVersion, "version-file"
	}
	if spec == "" && repo != nil {
		if v, ok := repo.Property("node"); ok && v != "" {
			spec, src = v, "repo"
		}
	}
	if spec == "" {
		if v, ok := in.props.String("node"); ok && v != "" {
			spec, src = v, "workspace"
		}
	}
	if spec == "" && in.nodeReg.Default != "" {
		spec, src = in.nodeReg.Default, "user"
	}

	if spec == "" {
		if in.ambientBin == "" {
			return nil, &output.ErrInfo{
				Code:    output.CodeNodeNotFound,
				Message: fmt.Sprintf("no node configured and %s not found on PATH", in.tool),
				Hint:    "run: barista node list; or pass --node <name|version>",
			}
		}
		p.nodeSrc = "ambient"
		p.bin = in.ambientBin
		p.viaCmd = in.goos == "windows" && isCmdShim(in.ambientBin)
		return p, nil
	}

	entry, _, e := in.nodeReg.Resolve(spec)
	if e != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: fmt.Sprintf("node %q (from %s) is not registered", spec, sourceDesc(src, in.filePath)),
			Hint:    "run: barista node list",
		}
	}
	p.nodeSpec = spec
	p.nodeName = entry.Name
	p.nodeVersion = entry.Version
	p.nodeHome = entry.Path
	p.nodeSrc = src
	if src == "version-file" {
		p.nodeFile = in.filePath
	}
	p.pathEntry = node.BinDirFor(in.goos, entry.Path)
	p.bin, p.viaCmd = node.ToolBinaryPath(in.goos, entry.Path, in.tool)
	return p, nil
}

func sourceDesc(src, filePath string) string {
	if src == "version-file" && filePath != "" {
		return filepath.Base(filePath)
	}
	return src
}

func isCmdShim(bin string) bool {
	lower := strings.ToLower(bin)
	return strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")
}
