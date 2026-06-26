package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

func preflightWarehouseAuth(config project.Config, provider warehouse.Provider, credentialStore credentials.Store) error {
	for _, diagnostic := range provider.ConfigDiagnostics(config.Warehouse) {
		if diagnostic.Message != "" {
			return errors.New(diagnostic.Message)
		}
	}

	credentialsPath, err := provider.CredentialPath(credentialStore, config.Warehouse.Auth)
	if err != nil {
		return fmt.Errorf("check warehouse authentication: %w", err)
	}

	info, err := os.Stat(credentialsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s authentication for warehouse.auth %q was not found at %s; run segmentstream warehouse auth login or segmentstream warehouse auth --service-account-key=<path>", provider.DisplayName(), config.Warehouse.Auth, credentialsPath)
		}
		return fmt.Errorf("check warehouse authentication at %s: %w", credentialsPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s authentication path %s is a directory; run segmentstream warehouse auth login or segmentstream warehouse auth --service-account-key=<path>", provider.DisplayName(), credentialsPath)
	}

	return nil
}
