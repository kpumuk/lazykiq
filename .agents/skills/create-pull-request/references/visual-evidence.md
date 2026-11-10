# Visual evidence

Use this reference for every visual change. Capture the actual Lazykiq UI with VHS, including static screenshots. `doc/demo.vhs` is the style source of truth; [the starter tape](../assets/evidence.vhs) copies its current settings. Refresh those settings if the demo changes rather than sourcing the whole demo and replaying unrelated actions.

## Prepare the scene

Build separate baseline/proposed binaries from the pinned revisions. Keep tapes, binaries, fixtures, and outputs in a task-specific temporary directory; do not overwrite `doc/assets` or commit PR-only media. The starter uses `/tmp/lazykiq-pr-evidence` for illustration; choose a unique directory per task and update its paths.

Use the repository's `demo/` setup and `mise run demo-sidekiq80`, `mise run demo-sidekiq81`, or another documented task when suitable. Check `README.md` and `mise.toml` for the current version-to-Redis mapping; the starter's DB 1 matches the existing `doc/demo.vhs`. Record the selected version, edition, Redis URL, fixture, and setup commands. Edition-specific behavior needs the corresponding Sidekiq edition and observable state; a local simulation must be identified and does not prove licensed integration behavior.

Use reproducible fixture data. For before/after captures, reset the disposable scene to equivalent data and queue/process state before each run, especially after pausing, resuming, or deleting jobs. Keep terminal geometry and styling identical across the pair. Change dimensions only when the issue requires a particular viewport, and state those dimensions in the reproduction.

Check VHS, `ttyd`, `ffmpeg`, its browser runtime, and the exact Nerd Font are available. Missing fonts must not silently fall back to a different appearance. Run the tape from the repository root, pointing its launch command at the binary for the stated revision. Do not rely on `go run` compiling within a short startup sleep.

For normal UI captures, launch the binary with `env -u NO_COLOR COLORTERM=truecolor <binary> ...`, as in the starter tape. VHS can set `NO_COLOR=1` inside its shell, which disables Lazykiq's application colors even when `Set Theme` matches the demo. Apply the same color environment to both revisions; do not change application code just to force colors in a recording.

## Record

Copy and adapt the starter tape. Keep all `Set` directives before `Env`, typing, or other executable commands; VHS can ignore later settings and use its default geometry/font/theme. Replace its launch path, outputs, readiness expression, navigation, and capture points with the minimal scenario. Validate and render the adapted tape:

```bash
vhs validate /tmp/lazykiq-pr-evidence/after.vhs
vhs /tmp/lazykiq-pr-evidence/after.vhs
```

Use `Wait+Screen /.../` for a reliable visible readiness condition and short pauses for readability. Hide setup and compilation. Keep the relevant interaction and state changes visible: do not put scrolling, dialog transitions, pause/resume actions, or asynchronous results inside `Hide`/`Show` gaps. Include an initial state, the triggering input, and enough settled output to verify the result.

For static changes, use VHS `Screenshot` to capture a PNG at the relevant state; the starter's MP4 can remain an unuploaded byproduct. For dynamic changes, use VHS `Output ...mp4` and show the complete interaction. Capture both the failing baseline and corrected behavior for bugs, labeled with their revisions. A feature needs a recording of the proposed interaction, without a fabricated failing baseline. Show both pause and resume when both are affected, or opening, using, and closing a dialog when those transitions are affected.

Inspect each PNG and play the full recording. Confirm the metrics bar, selected row, and focused border show their themed colors, rather than uniformly using the terminal foreground. If normal UI output is monochrome, check `NO_COLOR` inside the VHS shell, correct the launch environment, and recapture before attaching files. Check the right build is running, the issue is visible, text/glyphs are readable, the relevant rows/scrollbar/dialog are in frame, and there are no startup errors. Shorten an oversized recording or encode it more compactly while retaining the evidence and style. GitHub's file-size limits depend on file type and account plan; check the [current attachment limits](https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/attaching-files) if necessary.

## Attach to the PR

Choose the media publication route by execution environment. Capture and inspection requirements are the same for both routes.

### Amp orbs

Publish media with `amp.public_artifact_url`. GitHub rejects image and video uploads made with Amp's GitHub App token; see [How Orbs Authenticate to GitHub](https://ampcode.com/docs/github#how-orbs-authenticate-to-github). Use Amp's public media route directly instead of trying `gh --attach`, custom upload endpoints, browser sign-in, or a separate CLI login in the orb. Continue to create and edit the PR with `gh` and `--body-file`, without `--attach`.

Discover `amp.public_artifact_url` with `tool_search`, then call it through `code_exec` with the absolute paths of the inspected media. For example, pass:

```json
{"paths":["/tmp/lazykiq-pr-evidence/before.png","/tmp/lazykiq-pr-evidence/after.png"]}
```

Embed each returned public image URL in the prepared body, using the returned URLs verbatim rather than guessing them:

```markdown
![Before: the queue name is clipped](<returned-before-image-url>)
![After: the queue name and new column remain readable](<returned-after-image-url>)
```

GitHub does not play externally hosted videos inline. For a recording, also publish an animated GIF derived from the inspected VHS recording, embed the GIF as the preview, and link the full video:

```markdown
![After: scrolling keeps the selected job visible](<returned-public-gif-url>)
[Watch the full recording](<returned-public-video-url>)
```

Published URLs are public to anyone with the link and remain available until unpublished. Check that media contains no secrets or private customer data before publishing, and retain it while the PR needs it. Amp-hosted media satisfies this skill's evidence requirement; unpublished thread attachments do not.

### Personal laptops and other local checkouts

Use the local GitHub CLI authentication to upload media to GitHub. Check `gh pr create --help` / `gh pr edit --help` for `--attach`. GitHub CLI supports uploading local images/videos and rewriting matching local references in a `--body-file` into GitHub asset URLs; see [the official attachment guide](https://docs.github.com/en/github-cli/github-cli/attaching-files-with-github-cli).

For example, place the PNG under the intended heading in the prepared body:

```markdown
![After: the queue name and new column remain readable](/tmp/lazykiq-pr-evidence/after.png)
```

For video, put the reference in its own paragraph so it becomes an inline player:

```markdown
![](/tmp/lazykiq-pr-evidence/after.mp4)
```

Then pass the same exact paths to `--attach`, repeating it for before/after assets or multiple scenes. An illustrative creation command is:

```bash
gh pr create --repo kpumuk/lazykiq --base main --head <branch> \
  --title '<prepared title>' \
  --body-file /tmp/lazykiq-pr-evidence/pr-body.md \
  --attach /tmp/lazykiq-pr-evidence/after.mp4
```

Substitute the resolved repository, base, head, and prepared paths; do not assume `main` or the example paths. For an existing PR, use `gh pr edit <url> --body-file ... --attach ...`. Preserve already uploaded asset URLs on updates and attach only new files.

If the installed CLI or its credential cannot upload, retain the files and report the exact blocker. Do not invent an API upload endpoint or substitute VHS's public hosting service.

### Verify publication

A nonzero `gh` exit can still mean the PR was created or updated with only some attachments. Inspect the returned URL and current body before retrying. Repair missing media on that PR, preserving successful uploads; do not create a duplicate PR. After successful creation, register the PR with the Codex app's `attach_artifact` tool when available, including when an upload repair remains pending. Read back the final body to ensure there are no local-only references. Verify published media is accessible without an Amp sign-in, then inspect the rendered images, animated previews, and recording links or players.
