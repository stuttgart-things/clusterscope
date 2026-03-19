// Package repos defines the repos.yaml configuration schema for the git-sync
// sidecar. The file is mounted as a ConfigMap in Kubernetes and describes
// which Git repositories and paths clusterscope should scan.
package repos

// Config is the top-level structure of repos.yaml.
type Config struct {
	Repos []Entry `yaml:"repos"`
}

// Entry describes a single Git repository to sync.
type Entry struct {
	// Name is a human-readable identifier used as the subdirectory name under -root.
	Name string `yaml:"name"`
	// URL is the HTTPS or SSH clone URL of the repository.
	URL string `yaml:"url"`
	// Branch is the branch to track (default: main).
	Branch string `yaml:"branch,omitempty"`
	// Paths is a list of sub-paths within the repository to scan.
	// If empty, the entire repo root is scanned.
	Paths []string `yaml:"paths,omitempty"`
	// SyncInterval controls how often git-sync polls (e.g. "60s").
	SyncInterval string `yaml:"syncInterval,omitempty"`
	// Auth configures optional authentication for private repositories.
	Auth *Auth `yaml:"auth,omitempty"`
}

// Auth describes how to authenticate against a private Git repository.
type Auth struct {
	// Type is "token" (HTTPS Bearer / GitHub PAT) or "ssh".
	Type string `yaml:"type"`
	// SecretRef is the name of the Kubernetes Secret in the same namespace
	// that holds the credentials.
	// For type=token: key "token"
	// For type=ssh:   key "ssh-privatekey"
	SecretRef string `yaml:"secretRef"`
}
