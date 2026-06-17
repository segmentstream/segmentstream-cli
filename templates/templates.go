package templates

import "embed"

//go:embed project/Dockerfile project/README.md project/dbt_project.yml project/docker-compose.yml project/profiles.yml project/dagster/definitions.py
var Project embed.FS
