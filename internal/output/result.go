package output

const SchemaVersion = 1

type Status string

const (
	StatusOK      Status = "ok"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

const (
	CodeRepoExists              = "REPO_EXISTS"
	CodeRepoNotFound            = "REPO_NOT_FOUND"
	CodeRepoDeleteFailed        = "REPO_DELETE_FAILED"
	CodeNotCloned               = "NOT_CLONED"
	CodeDirtyWorktree           = "DIRTY_WORKTREE"
	CodeNothingToPush           = "NOTHING_TO_PUSH"
	CodeDefaultBranchUnresolved = "DEFAULT_BRANCH_UNRESOLVED"
	CodeGitError                = "GIT_ERROR"
	CodeConfigError             = "CONFIG_ERROR"
	CodeNoMatchingRepos         = "NO_MATCHING_REPOS"
	CodeWorkspaceNotFound       = "WORKSPACE_NOT_FOUND"
	CodeJDKExists               = "JDK_EXISTS"
	CodeJDKNotFound             = "JDK_NOT_FOUND"
	CodeNotAJDK                 = "NOT_A_JDK"
	CodeJDKProbeFailed          = "JDK_PROBE_FAILED"
	CodeJDKMajorMismatch        = "JDK_MAJOR_MISMATCH"
	CodeJDKUnsupportedDistro    = "JDK_UNSUPPORTED_DISTRO"
	CodeJDKUnsupportedPlatform  = "JDK_UNSUPPORTED_PLATFORM"
	CodeJDKDownloadFailed       = "JDK_DOWNLOAD_FAILED"
	CodeJDKInstallFailed        = "JDK_INSTALL_FAILED"
	CodeJDKNotManaged           = "JDK_NOT_MANAGED"
	CodeJDKUninstallFailed      = "JDK_UNINSTALL_FAILED"
	CodeJDKAvailableFailed      = "JDK_AVAILABLE_FAILED"
	CodeJDKChecksumMismatch     = "JDK_CHECKSUM_MISMATCH"
	CodeMavenExists             = "MAVEN_EXISTS"
	CodeMavenNotFound           = "MAVEN_NOT_FOUND"
	CodeNotAMaven               = "NOT_A_MAVEN"
	CodeMavenProbeFailed        = "MAVEN_PROBE_FAILED"
	CodeMavenDownloadFailed     = "MAVEN_DOWNLOAD_FAILED"
	CodeMavenChecksumMismatch   = "MAVEN_CHECKSUM_MISMATCH"
	CodeMavenInstallFailed      = "MAVEN_INSTALL_FAILED"
	CodeMavenNotManaged         = "MAVEN_NOT_MANAGED"
	CodeMavenUninstallFailed    = "MAVEN_UNINSTALL_FAILED"
	CodeMavenAvailableFailed    = "MAVEN_AVAILABLE_FAILED"
	CodeMavenExecFailed         = "MAVEN_EXEC_FAILED"
	CodeJavaExecFailed          = "JAVA_EXEC_FAILED"
	CodeGradleExists            = "GRADLE_EXISTS"
	CodeGradleNotFound          = "GRADLE_NOT_FOUND"
	CodeNotAGradle              = "NOT_A_GRADLE"
	CodeGradleProbeFailed       = "GRADLE_PROBE_FAILED"
	CodeGradleDownloadFailed    = "GRADLE_DOWNLOAD_FAILED"
	CodeGradleChecksumMismatch  = "GRADLE_CHECKSUM_MISMATCH"
	CodeGradleInstallFailed     = "GRADLE_INSTALL_FAILED"
	CodeGradleNotManaged        = "GRADLE_NOT_MANAGED"
	CodeGradleUninstallFailed   = "GRADLE_UNINSTALL_FAILED"
	CodeGradleAvailableFailed   = "GRADLE_AVAILABLE_FAILED"
	CodeGradleExecFailed        = "GRADLE_EXEC_FAILED"
	CodeRepoRemoteMismatch      = "REPO_REMOTE_MISMATCH"
	CodeUpgradeCheckFailed      = "UPGRADE_CHECK_FAILED"
	CodeUpgradeDownloadFailed   = "UPGRADE_DOWNLOAD_FAILED"
	CodeUpgradeChecksumMismatch = "UPGRADE_CHECKSUM_MISMATCH"
	CodeUpgradeReplaceFailed    = "UPGRADE_REPLACE_FAILED"
	CodeUsageError              = "USAGE_ERROR"
	CodeConfirmationRequired    = "CONFIRMATION_REQUIRED"
)

type ErrInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Result struct {
	Name       string         `json:"name"`
	Path       string         `json:"path"`
	Status     Status         `json:"status"`
	Action     string         `json:"action"`
	Branch     string         `json:"branch,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	Error      *ErrInfo       `json:"error,omitempty"`
	DurationMs int64          `json:"durationMs"`
}

func (r Result) Changed() bool {
	b, _ := r.Detail["changed"].(bool)
	return b
}

type Summary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func Summarize(results []Result) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			s.OK++
		case StatusSkipped:
			s.Skipped++
		case StatusFailed:
			s.Failed++
		}
	}
	return s
}

func ExitCode(results []Result) int {
	for _, r := range results {
		if r.Status == StatusFailed {
			return 1
		}
	}
	return 0
}

type Envelope struct {
	SchemaVersion int      `json:"schemaVersion"`
	Command       string   `json:"command"`
	Workspace     string   `json:"workspace,omitempty"`
	Success       bool     `json:"success"`
	Summary       *Summary `json:"summary,omitempty"`
	Results       []Result `json:"results,omitempty"`
	Error         *ErrInfo `json:"error,omitempty"`
}

func NewEnvelope(command, workspace string, results []Result) Envelope {
	s := Summarize(results)
	return Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Workspace:     workspace,
		Success:       s.Failed == 0,
		Summary:       &s,
		Results:       results,
	}
}

func ErrorEnvelope(command string, e *ErrInfo) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Success:       false,
		Error:         e,
	}
}
