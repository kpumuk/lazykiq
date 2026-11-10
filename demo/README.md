# Sidekiq Simulation

A demo environment for testing Sidekiq TUI tools. Generates continuous job traffic across multiple queues with realistic failure scenarios.

The current profile is intentionally lightweight: older Sidekiq versions stay compact, while the 8.1 demo uses five thin worker processes to stretch the process-oriented UI without bringing back the original CPU load.

## Quick Start

```bash
mise run demo
mise run demo-sidekiq73
mise run demo-sidekiq80
mise run demo-sidekiq81
```

`mise run demo` starts the latest Sidekiq demo (8.1) on `http://localhost:9292` with `redis://localhost:6379/0`.
`mise run demo-sidekiq73` starts Sidekiq 7.3 on `http://localhost:9294` with `redis://localhost:6379/2`.
`mise run demo-sidekiq80` starts Sidekiq 8.0 on `http://localhost:9293` with `redis://localhost:6379/1`.
`mise run demo-sidekiq81` starts Sidekiq 8.1 on `http://localhost:9292` with `redis://localhost:6379/0`.

The three tasks can run in parallel because each demo stand uses its own Redis database and dashboard port.

## Structure

**10 Base Jobs** with varying performance (10-1000ms):

| Job | Queue | Fail Rate | Retries |
|-----|-------|-----------|---------|
| NotificationJob | critical | 0% | 3 |
| PaymentProcessingJob | critical | 5% | 2 |
| ImageProcessingJob | default | 12% | 0 (→ dead) |
| WebhookDeliveryJob | default | 10% | 2 |
| EmailDeliveryJob | mailers | 0% | 3 |
| ReportGenerationJob | batch | 0% | 1 |
| DataSyncJob (iterable) | batch | 8% | 1 |
| AnalyticsJob | low | 0% | 1 |
| CacheWarmupJob | low | 0% | 1 |
| CleanupJob | low | 0% | 0 |

**5 Queues** (by priority): critical, default, mailers, batch, low

**Limits**: 1.5k jobs/queue, 1k retry queue max, 500 scheduled max

**Extra cases**:
- ActiveJob-wrapped jobs with GlobalID-serialized arguments
- ActionMailer-wrapped jobs with arguments (including GlobalID)
- Tagged jobs sharing the same 5 tags (intersection)
- **DataSyncJob** uses `Sidekiq::IterableJob` to process entity IDs one at a time:
  - Iterates over 1-10 entity IDs per job
  - Random sleep duration per iteration (0.2-1.0s)
  - 8% failure rate per iteration for realistic retry behavior
  - Demonstrates job interruption and resumption on worker restart

## Development and dependency updates

`Gemfile` and `Gemfile.lock` contain the auditing and formatting tools. Each runtime has its own `bundles/sidekiq-{7.3,8.0,8.1}/Gemfile` and `Gemfile.lock`, so the demos can retain different Sidekiq versions.

Local tooling, CI, and Docker use Ruby 4.0, with exact patch versions pinned in `.ruby-version` and `Dockerfile`. Dependabot updates the Docker image's tag and digest within 4.0.x. After its test workflow finishes, `sync demo Ruby` copies a new patch version into `.ruby-version` on the same PR. Digest-only updates need no extra commit. Major and minor Ruby upgrades remain manual.

To install the tooling and one runtime locally, run these commands from `demo/`:

```bash
mise exec ruby -- bundle install
BUNDLE_GEMFILE=bundles/sidekiq-8.1/Gemfile mise exec ruby -- bundle install
```

Set the same `BUNDLE_GEMFILE` when running a demo process outside Docker. The existing `mise run demo*` commands select their bundles automatically. Run `mise run deps-ruby` to update all four bundles within their declared ranges.

Dependabot groups runtime dependency updates across the three bundles, with separate pull requests for routine and security updates. Tooling updates stay separate. Sidekiq receives only patch updates within 7.3, 8.0, and 8.1: the three-component version constraints and `lockfile-only` strategy prevent automatic release-line changes. Fixes outside any manifest range require a manual manifest change; vulnerability alerts remain visible even when an automatic fix is blocked.

The test workflow installs all four bundles in frozen mode and audits their lockfiles. Ruby CodeQL analyzes the demo source, including the web setup in `web.rb`; it does not replace the dependency advisory audit.

## Ruby synchronization setup

The synchronization workflow needs a GitHub App installation token so its commit triggers normal PR checks. The built-in `GITHUB_TOKEN` does not provide unattended follow-up checks. Configure the App before relying on automated pin updates:

1. In your account's **Settings → Developer settings → GitHub Apps**, create a private App. Use this repository's URL as its homepage and disable the webhook. No callback URL or event subscriptions are needed.
2. Grant repository permissions **Contents: Read and write** and **Pull requests: Read-only**. Leave other permissions unchanged; GitHub adds read-only metadata access. Install the App only on `kpumuk/lazykiq`.
3. Copy its **Client ID** into the repository's **Settings → Secrets and variables → Actions → Variables** as `DEPENDABOT_SYNC_APP_CLIENT_ID`.
4. Generate a private key on the App's settings page. Store the downloaded PEM file's full contents as the repository **Actions secret** `DEPENDABOT_SYNC_APP_PRIVATE_KEY`. Do not put it in Dependabot secrets, commit it, or share it in an issue or chat. The workflow creates a short-lived installation token; there is no personal access token to maintain.
5. In the repository's **Settings → Actions → General**, add `actions/create-github-app-token@*` to the allowed actions. Keep the full-commit-SHA requirement; the workflow pins the action to a reviewed revision.

The workflow becomes active only when it is on the default branch. It reads current PR data with a read-only token and creates the App token only when a pin update is needed. It accepts only open, same-repository Dependabot PRs against `main` that modify `demo/Dockerfile` and optionally `.ruby-version`. It runs default-branch code, never PR code or downloaded artifacts. It rejects release-line changes, downgrades, and stale runs, and never force-pushes. A `[dependabot skip]` commit allows Dependabot to replace and regenerate the synchronization after a rebase. Missing setup prevents synchronization, not the original Dependabot image update; maintain `.ruby-version` manually until setup is complete.

CI rejects mismatched Ruby pins. A new Dependabot patch PR initially fails that check; synchronization runs after the test workflow completes, even if it failed. After setup and merge, verify the next Dependabot Ruby patch PR: its completed `test` run should trigger `sync demo Ruby`, append a commit changing only `demo/.ruby-version`, and start a new set of PR checks. A digest-only update should finish without an extra commit. To run the helper and pin-consistency tests locally, use `node --test .github/scripts/*.test.mjs` from the repository root.

## Files

```
├── start.rb          # Entry point
├── scheduler.rb      # Job scheduling logic
├── boot.rb           # Sidekiq configuration
├── config.ru         # Rack entry point
├── web.rb            # Web UI and session middleware
├── bundles/          # Independent Sidekiq runtime bundles
└── lib/jobs/         # Job classes
```
