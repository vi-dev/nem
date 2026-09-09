package spec

type Package struct {
	Schema           int
	Name             string
	Description      string
	Homepage         string
	License          string
	Platforms        []Platform
	Deps             []Dep
	Versions         []VersionEntry
	VersionDiscovery *Discovery
	Artifact         Artifact
	Install          []Action
	Bins             []string
	Libs             []string
	Env              []EnvExport
	Build            *Build
	Test             []TestStep
}

type DepKind string

const (
	DepKindRun  DepKind = "run"
	DepKindLink DepKind = "link"
)

type Dep struct {
	Name, Version string
	Platforms     []Platform
	Kind          DepKind
	Compat        string
}

type VersionEntry struct {
	Version      string
	Sha256       map[string]string
	SourceSha256 string
}

type Discovery struct {
	GitHub *GitHubDiscovery
	GitLab *GitLabDiscovery
	Git    *GitDiscovery
	HTTP   *HTTPDiscovery
	OCI    string
}

type GitHubDiscovery struct{ Repo, Filter, Prefix, Suffix string }

type GitLabDiscovery struct{ Repo, Filter, Prefix, Suffix string }

type GitDiscovery struct{ URL, Filter, Prefix, Suffix string }

type HTTPDiscovery struct{ URL, Filter string }

type Artifact struct {
	URL    string
	GitHub *GitHubAsset
	OCI    string
}

type GitHubAsset struct{ Repo, Asset string }

type Action struct {
	Extract   *ExtractAction
	Copy      *CopyAction
	Move      *MoveAction
	Mkdir     string
	Platforms []Platform
}

type ExtractAction struct{ Strip int }

type CopyAction struct {
	Src, Dst string
	Mode     uint32
}

type MoveAction struct{ Src, Dst string }

type EnvExport struct {
	Name, Value string
	Platforms   []Platform
}

type Build struct {
	Deps      []Dep
	Source    struct{ URL string }
	Steps     []BuildStep
	Output    string
	Normalize *bool
}

type BuildStep struct {
	Run       string
	Platforms []Platform
}

type TestStep struct {
	Run       string
	Platforms []Platform
}
