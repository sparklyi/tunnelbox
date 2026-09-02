package service

import "context"

type Workspace struct {
	ID                   string
	Name                 string
	AccountID            string
	ZoneID               string
	CloudflareTokenPath  string
	AdminTokenPath       string
}

// WorkspaceRepository is the narrow settings contract consumed by the
// Cloudflare integration. Secrets are represented only by their file path.
type WorkspaceRepository interface {
	GetWorkspace(context.Context, string) (Workspace, error)
	SaveCloudflareConfig(context.Context, string, string, string, string) error
}
