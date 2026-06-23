package initflow

import (
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/internal/source"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
)

type ProjectStore interface {
	LoadPartial() (project.Config, bool, error)
	SelectWarehouse(warehouseType, defaultAuthName string) (project.Config, error)
}

type CredentialStore interface {
	HasCredential(warehouseType, name string) (bool, error)
	HasMatchingAccessMarker(warehouseType, name string, config project.Warehouse) (bool, error)
}

type SourceVerificationStatus struct {
	Valid  bool
	Reason string
}

type SourceVerifier interface {
	CheckSource(projectRoot string, source project.Source) (SourceVerificationStatus, error)
}

type ProjectScaffolder interface {
	EnsureInitFiles() error
}

type projectScaffolder struct {
	Root string
}

type sourceVerifier struct{}

type providerCredentialStore struct {
	Store    credentials.Store
	Registry warehouse.Registry
}

func (verifier sourceVerifier) CheckSource(projectRoot string, source project.Source) (SourceVerificationStatus, error) {
	status, err := sourcepkg.Check(projectRoot, source)
	if err != nil {
		return SourceVerificationStatus{}, err
	}
	return SourceVerificationStatus{
		Valid:  status.Valid,
		Reason: status.Reason,
	}, nil
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
	if service.CredentialStore != nil {
		return service.CredentialStore
	}
	return providerCredentialStore{
		Store:    service.Credentials,
		Registry: service.WarehouseRegistry,
	}
}

func (service Service) projectScaffolder() ProjectScaffolder {
	if service.Scaffolder != nil {
		return service.Scaffolder
	}
	return projectScaffolder{Root: service.ProjectRoot}
}

func (service Service) sourceVerifier() SourceVerifier {
	if service.SourceVerifier != nil {
		return service.SourceVerifier
	}
	return sourceVerifier{}
}

func (store providerCredentialStore) HasCredential(warehouseType, name string) (bool, error) {
	provider, err := store.Registry.Provider(warehouseType)
	if err != nil {
		return false, err
	}
	return provider.HasCredential(store.Store, name)
}

func (store providerCredentialStore) HasMatchingAccessMarker(warehouseType, name string, config project.Warehouse) (bool, error) {
	provider, err := store.Registry.Provider(warehouseType)
	if err != nil {
		return false, err
	}
	return provider.HasMatchingAccessMarker(store.Store, name, config)
}
