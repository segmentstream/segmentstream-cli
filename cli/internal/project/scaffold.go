package project

type ScaffoldResult struct {
	ConfigCreated     bool
	ConfigExisted     bool
	ReadmeCreated     bool
	AgentGuideCreated bool
}

func Scaffold(root string) (ScaffoldResult, error) {
	var result ScaffoldResult
	store := Store{Root: root}

	exists, err := store.Exists()
	if err != nil {
		return result, err
	}
	if exists {
		result.ConfigExisted = true
	} else {
		if err := store.WriteDefault(); err != nil {
			return result, err
		}
		result.ConfigCreated = true
	}

	if err := EnsureRuntimeGitignored(root); err != nil {
		return result, err
	}

	created, err := EnsureProjectReadme(root)
	if err != nil {
		return result, err
	}
	result.ReadmeCreated = created

	created, err = EnsureAgentGuide(root)
	if err != nil {
		return result, err
	}
	result.AgentGuideCreated = created

	return result, nil
}
