package initflow

import (
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

type ProjectStore interface {
	LoadPartial() (project.Config, bool, error)
	SelectWarehouse(warehouseType string) (project.Config, error)
}

type CredentialStore interface {
	HasBigQueryCredential(name string) (bool, error)
	HasMatchingAccessMarker(name, projectID, dataset, location string) (bool, error)
}

type ProjectScaffolder interface {
	EnsureInitFiles() error
}

type projectScaffolder struct {
	Root string
}

func (scaffolder projectScaffolder) EnsureInitFiles() error {
	if err := project.EnsureRuntimeGitignored(scaffolder.Root); err != nil {
		return err
	}
	if _, err := project.EnsureProjectReadme(scaffolder.Root); err != nil {
		return err
	}
	if _, err := project.EnsureAgentGuide(scaffolder.Root); err != nil {
		return err
	}
	return nil
}

func (service Service) projectStore() ProjectStore {
	if service.ProjectStore != nil {
		return service.ProjectStore
	}
	return project.Store{Root: service.ProjectRoot}
}

func (service Service) credentialStore() CredentialStore {
	if service.Credentials != nil {
		return service.Credentials
	}
	return credentials.Store{}
}

func (service Service) projectScaffolder() ProjectScaffolder {
	if service.Scaffolder != nil {
		return service.Scaffolder
	}
	return projectScaffolder{Root: service.ProjectRoot}
}
