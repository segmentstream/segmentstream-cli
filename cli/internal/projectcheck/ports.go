package projectcheck

import (
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/cli/internal/source"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

type ProjectStore interface {
	LoadPartial() (project.Config, bool, error)
}

type CredentialStore interface {
	HasCredential(warehouseType, name string) (bool, error)
	HasMatchingAccessMarker(warehouseType, name string, config project.Warehouse) (bool, error)
}

type SourceVerificationStatus struct {
	Valid    bool
	Reason   string
	Contract sourcepkg.ContractIdentity
}

type SourceVerifier interface {
	CheckSource(projectRoot string, source project.Source) (SourceVerificationStatus, error)
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
		Valid:    status.Valid,
		Reason:   status.Reason,
		Contract: status.Contract,
	}, nil
}

func (evaluator Evaluator) projectStore() ProjectStore {
	if evaluator.ProjectStore != nil {
		return evaluator.ProjectStore
	}
	return project.Store{Root: evaluator.ProjectRoot}
}

func (evaluator Evaluator) credentialStore() CredentialStore {
	if evaluator.CredentialStore != nil {
		return evaluator.CredentialStore
	}
	return providerCredentialStore{
		Store:    evaluator.Credentials,
		Registry: evaluator.WarehouseRegistry,
	}
}

func (evaluator Evaluator) sourceVerifier() SourceVerifier {
	if evaluator.SourceVerifier != nil {
		return evaluator.SourceVerifier
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
