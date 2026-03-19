// Package render generates HTML output from a graph.ClusterProfile.
package render

import (
	"encoding/json"
	"html/template"
	"io"
	"sort"
	"strings"

	_ "embed"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

//go:embed template.html
var htmlTemplate string

//go:embed shell.html
var shellTemplate string

//go:embed index.html
var indexTemplate string

// IndexCluster is one row in the multi-cluster index page.
type IndexCluster struct {
	Name    string
	Profile *graph.ClusterProfile
	Err     error
}

type indexData struct {
	Clusters    []IndexCluster
	TotalNodes  int
	TechSummary string
}

type kustCard struct {
	Name      string
	Path      string
	Version   string
	Domain    string
	DependsOn []string
}

type argoAppCard struct {
	Name     string
	Project  string
	RepoURL  string
	Path     string
	Revision string
	DestNS   string
}

type argoAppSetCard struct {
	Name     string
	Project  string
	RepoURL  string
	Paths    string
	Revision string
}

type argoProjectCard struct {
	Name        string
	Description string
}

type gitRepoCard struct {
	Name     string
	URL      string
	Branch   string
	Interval string
}

type fluxInstanceData struct {
	Version    string
	SyncURL    string
	SyncPath   string
	SyncRef    string
	Components []string
}

// Package-level build info, set once at startup via SetBuildInfo.
var (
	appVersion  = "dev"
	buildCommit = "unknown"
	buildDate   = "unknown"
)

// SetBuildInfo stores build-time version metadata for use in rendered pages.
func SetBuildInfo(version, commit, date string) {
	appVersion = version
	buildCommit = commit
	buildDate = date
}

type templateData struct {
	ClusterName   string
	ClusterPath   string
	Technology    string
	Meta          map[string]string
	GraphDataJSON template.JS

	FluxInstance          *fluxInstanceData
	FluxSops              bool
	GitRepos              []gitRepoCard
	Kustomizations        []kustCard
	InfraKustomizations   []kustCard
	AppKustomizations     []kustCard
	HomerunKustomizations []kustCard

	HasSources     bool
	HasControllers bool
	HasInfra       bool
	HasApps        bool
	HasHomerun     bool

	// ArgoCD-specific
	ArgoProjects    []argoProjectCard
	ArgoAppSets     []argoAppSetCard
	ArgoApps        []argoAppCard
	HasArgoProjects bool
	HasArgoAppSets  bool
	HasArgoApps     bool

	// Live mode
	LiveRefreshed string // non-empty when -live was used

	// Build info
	AppVersion  string
	BuildCommit string
	BuildDate   string
}

// WriteHTML renders the cluster profile as a standalone HTML page to w.
func WriteHTML(w io.Writer, profile *graph.ClusterProfile) error {
	jsonBytes, err := json.Marshal(profile.Graph)
	if err != nil {
		return err
	}
	td := buildTemplateData(profile, template.JS(jsonBytes))
	tmpl, err := template.New("clusterscope").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, td)
}

func buildTemplateData(profile *graph.ClusterProfile, graphJSON template.JS) templateData {
	td := templateData{
		ClusterName:   profile.ClusterName,
		ClusterPath:   profile.ClusterPath,
		Technology:    profile.Technology,
		Meta:          profile.Meta,
		GraphDataJSON: graphJSON,
		AppVersion:    appVersion,
		BuildCommit:   buildCommit,
		BuildDate:     buildDate,
	}

	for _, n := range profile.Graph.Nodes {
		switch n.Type {
		case "source":
			branch, interval := parseSub(n.Sub)
			td.GitRepos = append(td.GitRepos, gitRepoCard{
				Name:     n.ID,
				URL:      n.Path,
				Branch:   branch,
				Interval: interval,
			})
		case "infra":
			c := nodeToCard(n)
			td.Kustomizations = append(td.Kustomizations, c)
			td.InfraKustomizations = append(td.InfraKustomizations, c)
		case "homerun2", "profile":
			c := nodeToCard(n)
			td.Kustomizations = append(td.Kustomizations, c)
			td.HomerunKustomizations = append(td.HomerunKustomizations, c)
		case "argoproject":
			td.ArgoProjects = append(td.ArgoProjects, argoProjectCard{
				Name:        n.ID,
				Description: n.Sub,
			})
		case "argoappset":
			td.ArgoAppSets = append(td.ArgoAppSets, argoAppSetCard{
				Name:     n.ID,
				RepoURL:  n.Source,
				Paths:    n.Sub,
				Revision: n.Version,
			})
		case "argoapp":
			td.ArgoApps = append(td.ArgoApps, argoAppCard{
				Name:     n.ID,
				RepoURL:  n.Source,
				Path:     n.Path,
				Revision: n.Version,
				DestNS:   n.Domain,
			})
		default:
			c := nodeToCard(n)
			td.Kustomizations = append(td.Kustomizations, c)
			td.AppKustomizations = append(td.AppKustomizations, c)
		}
	}

	for _, sl := range []*[]kustCard{
		&td.InfraKustomizations,
		&td.AppKustomizations,
		&td.HomerunKustomizations,
		&td.Kustomizations,
	} {
		sort.Slice(*sl, func(i, j int) bool { return (*sl)[i].Name < (*sl)[j].Name })
	}

	td.HasSources = len(td.GitRepos) > 0
	td.HasInfra = len(td.InfraKustomizations) > 0
	td.HasApps = len(td.AppKustomizations) > 0
	td.HasHomerun = len(td.HomerunKustomizations) > 0
	td.HasArgoProjects = len(td.ArgoProjects) > 0
	td.HasArgoAppSets = len(td.ArgoAppSets) > 0
	td.HasArgoApps = len(td.ArgoApps) > 0

	if profile.Technology == "flux" {
		fi := &fluxInstanceData{
			Version:  profile.Meta["fluxVersion"],
			SyncURL:  profile.Meta["fluxSyncURL"],
			SyncPath: profile.Meta["fluxSyncPath"],
			SyncRef:  profile.Meta["fluxSyncRef"],
		}
		if raw := profile.Meta["fluxComponents"]; raw != "" {
			var comps []string
			if err := json.Unmarshal([]byte(raw), &comps); err == nil {
				fi.Components = comps
			}
		}
		if fi.Version != "" {
			td.FluxInstance = fi
			td.HasControllers = true
		}
		td.FluxSops = profile.Meta["sops"] == "true"
	}

	return td
}

func nodeToCard(n graph.Node) kustCard {
	return kustCard{
		Name:      n.ID,
		Path:      n.Path,
		Version:   n.Version,
		Domain:    n.Domain,
		DependsOn: n.DependsOn,
	}
}

func parseSub(sub string) (branch, interval string) {
	parts := strings.SplitN(sub, "·", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return sub, ""
}

// WriteShell renders the HTMX dashboard shell page to w.
func WriteShell(w io.Writer) error {
	type shellData struct {
		AppVersion  string
		BuildCommit string
		BuildDate   string
	}
	tmpl, err := template.New("shell").Parse(shellTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, shellData{
		AppVersion:  appVersion,
		BuildCommit: buildCommit,
		BuildDate:   buildDate,
	})
}

// WriteIndex renders a multi-cluster index page from a list of cluster entries.
func WriteIndex(w io.Writer, clusters []IndexCluster) error {
	total := 0
	techs := map[string]bool{}
	for _, c := range clusters {
		if c.Profile != nil {
			total += len(c.Profile.Graph.Nodes)
			techs[c.Profile.Technology] = true
		}
	}
	techNames := make([]string, 0, len(techs))
	for t := range techs {
		techNames = append(techNames, t)
	}
	sort.Strings(techNames)

	data := indexData{
		Clusters:    clusters,
		TotalNodes:  total,
		TechSummary: strings.Join(techNames, " + "),
	}

	tmpl, err := template.New("index").Parse(indexTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}
