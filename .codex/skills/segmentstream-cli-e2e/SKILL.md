---
name: segmentstream-cli-e2e
description: Run end-to-end verification for the SegmentStream CLI using a disposable local project, the gh CLI for issue context, and a real BigQuery warehouse. Use when asked to test SegmentStream CLI behavior end to end, reproduce a GitHub issue, validate agentic UX flows, create a scratch SegmentStream project, verify BigQuery warehouse setup in project segmentstream-ai-website, or compare current CLI behavior against an issue description.
---

# SegmentStream CLI E2E

## Scope

Use this skill to verify the CLI as an external user or agent would experience it. Prefer real CLI commands and JSON outputs over direct package calls. Keep the scratch project under `tests/`, which should be gitignored, and avoid destructive cleanup unless the user asks.

Do not add or assume a `segmentstream source test` command. That architecture was rejected. Source verification should be discussed before implementation.

## Workflow

1. Inspect repository state:

   ```sh
   git status --short
   ```

   Preserve unrelated changes. If `tests/` is not gitignored and you need a scratch project, add `tests/` to `.gitignore`.

2. Read GitHub issue context when relevant:

   ```sh
   gh issue list --limit 20
   gh issue view <number> --json title,body,comments,url,state,labels
   ```

   Use the issue body as the acceptance source, then compare it to actual CLI behavior.

3. Build a local CLI binary so tests use this checkout:

   ```sh
   mkdir -p /private/tmp/segmentstream-cli-e2e
   TMPDIR=/private/tmp GOCACHE=/private/tmp/segmentstream-go-build go build -o /private/tmp/segmentstream-cli-e2e/segmentstream ./cmd/segmentstream
   ```

4. Create a disposable project:

   ```sh
   mkdir -p tests/<scenario-name>
   cd tests/<scenario-name>
   /private/tmp/segmentstream-cli-e2e/segmentstream init --warehouse bigquery
   ```

5. Configure BigQuery in EU. Default to project `segmentstream-ai-website` unless the user gives another project. Use a unique dataset name such as `segmentstream_cli_e2e_<yyyymmdd>_<short_suffix>`.

   ```sh
   /private/tmp/segmentstream-cli-e2e/segmentstream warehouse configure \
     --project segmentstream-ai-website \
     --dataset <dataset> \
     --location EU \
     --create-dataset
   /private/tmp/segmentstream-cli-e2e/segmentstream warehouse test --json
   ```

   Commands that contact GitHub, BigQuery, OAuth, Docker, or other networked services need network/credential access. Request escalation before running them when the sandbox requires it.

6. Exercise agent-facing source flow:

   ```sh
   /private/tmp/segmentstream-cli-e2e/segmentstream init --json
   /private/tmp/segmentstream-cli-e2e/segmentstream source contracts --json
   /private/tmp/segmentstream-cli-e2e/segmentstream source contracts --type events --json
   /private/tmp/segmentstream-cli-e2e/segmentstream source scaffold ga4 --type events --json
   ```

   Check that scaffold output points to `IMPLEMENTATION_GUIDE.md`, not to a pretend-complete implementation. Inspect generated `sources/ga4/models/schema.yml`, `sources/ga4/source.yml`, `sources/ga4/models/events.sql`, and `sources/ga4/IMPLEMENTATION_GUIDE.md`.

7. If validating `segmentstream init` readiness, test both states:

   - With warehouse access verified and no sources, `init --json` should not say ready; it should point to `segmentstream source contracts`.
   - Once `segmentstream.yml` declares at least one source, readiness may proceed to `segmentstream run`. Do not invent a source-test marker gate.

8. Run Go tests after code changes:

   ```sh
   TMPDIR=/private/tmp GOCACHE=/private/tmp/segmentstream-go-build go test ./...
   ```

## BigQuery Guardrails

- Use EU datasets for this repo's E2E scratch work unless the user asks otherwise.
- Prefer creating a new unique dataset over reusing one when behavior may mutate warehouse state.
- Do not delete datasets or tables by default. Report created project/dataset names so the user can decide cleanup.
- If credentials are missing, stop and ask for auth direction instead of fabricating credentials.

## What To Report

Summarize:

- issue or scenario tested
- scratch project path
- BigQuery project, dataset, and location
- key commands run
- observed CLI behavior versus expected behavior
- code/docs changed, if any
- tests run and pass/fail status

Keep raw JSON snippets short. Prefer describing the important fields: `ready`, `next_action.command`, `diagnostics`, scaffold `actions`, and generated file paths.
