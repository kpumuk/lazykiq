---
name: create-pull-request
description: Prepare, create, or update Lazykiq pull requests with concise goal-first prose, verified bug reproductions, VHS screenshots or recordings for visual changes, and benchmark results for performance or memory optimizations.
---

# Create Pull Request

Use for a prepared, focused Lazykiq change. Keep each PR about one independently reviewable problem.

## Prepare

Read `AGENTS.md`, any contribution guide, and the current PR template if present. Inspect the actual diff and relevant conversation or issue for the goal. Infer intent only when supported by that evidence; ask a short clarification if the motivation remains materially unclear.

Preserve unrelated changes. Use flat branch names as required by `AGENTS.md`. Do not bundle unrelated fixes, create tickets, or rewrite existing commits merely to raise a PR.

Determine the repository, head, and intended base before collecting evidence. Prefer the user's explicit base, then the existing PR's base, then an explicit branch `gh-merge-base` configuration, then the remote default branch. A feature branch's tracking upstream does not establish the PR base. Refresh the relevant remote refs, pin the base and head SHAs, and inspect `git diff <base>...<head>` and the commits it contains. Resolve unexpected scope before publication.

If the change is uncommitted, make a focused commit when authorized, or record the exact working-tree patch used to collect evidence and replace that identifier with the matching final commit ID before publication. Do not label dirty-tree results as measurements of the unchanged `HEAD` commit.

For before/after comparisons, pin the unpatched comparison revision, normally the merge base, and explain any different choice. Use separate checkouts or binaries so evidence comes from the stated revisions without disturbing the working tree. Evidence must cover the final proposed code; refresh affected evidence after material changes.

## Required evidence

Requirements accumulate for mixed changes. Classify the behavior changed, rather than relying on the PR label or title.

| Change | Required in the PR body |
| --- | --- |
| Bug fix | Small, self-contained reproduction; verified failing and corrected results with revision IDs. |
| Static visual change: graph, table column, text, column width, spacing | VHS PNG screenshot; before and after for a bug fix, proposed appearance for a new feature. |
| Dynamic visual change: scrolling, scrollbar behavior, queue pause/resume, dialogs | VHS demo recording showing the interaction and result; before and after for a bug fix. Screenshots alone do not satisfy this requirement. |
| Performance or memory optimization | Reproducible benchmark results demonstrating the claimed improvement against the pinned baseline. |
| Simple refactoring or dependency update | No extra evidence by default. Visual changes need media; optimization work needs benchmarks; bug fixes still need a reproduction. |

All visual changes need media, including features and visual changes introduced by dependency updates or refactoring. A change affecting both static layout and interaction must demonstrate both; a clear recording can cover both without redundant screenshots.

Do not substitute test output, a golden file, a prose description, or a promised follow-up for required media or benchmark results. If a prerequisite is missing, complete independent preparation, report the precise blocker, and leave publication pending. Do not silently waive evidence by creating an incomplete draft PR.

## Reproduce bugs

Put the smallest runnable reproduction in the PR itself, so it stands alone without the chat or issue. Include necessary fixture/setup commands, exact launch command, relevant Go/Redis/Sidekiq versions and edition, and ordered UI actions or a direct API example. Specify terminal dimensions when they affect the bug. Avoid dumping a whole demo script when a few steps suffice.

Run the exact published steps against both the pinned unpatched revision and proposed change under equivalent conditions. Record the actual failure and corrected result, including exact output when relevant. Rerun any simplified example before publishing it. For visual bugs, the reproduction steps and attached before/after media jointly provide this evidence.

Never invent commands, APIs, outputs, errors, or benchmark numbers. Keep explanatory comments separate from captured output. A passing regression test alone is not a self-contained reproduction of the original problem.

## Capture visual evidence

Read [Visual evidence](references/visual-evidence.md) for capture and attachment instructions. Use VHS for both PNG screenshots and recordings. Read `doc/demo.vhs` at capture time and match its font, theme, colors, and rendering style; do not use desktop screenshots or a different terminal recorder.

Use a focused tape and a reproducible demo fixture. Show enough surrounding UI and state to make the result understandable. Inspect PNGs and play recordings before attaching them. Verify normal application colors are visible; a matching terminal theme alone does not prove color output is enabled. Follow the color setup in the visual evidence reference and recapture monochrome output before uploading. Attach the actual files to GitHub and embed the resulting assets in the PR body; local paths, committed binaries, or external hosting links do not satisfy the attachment requirement.

## Measure optimizations

Read [Benchmarks](references/benchmarks.md). Every performance or memory optimization needs a meaningful same-workload comparison against the pinned unpatched revision. Include the exact command and workload, environment, trial count, baseline and proposed medians, percentage deltas, and material limitations in a separate `## Benchmarks` section.

Measure the claimed property: time/throughput for speed, allocations/bytes or retained heap/RSS for the corresponding memory claim. If the benchmark does not demonstrate the optimization, investigate or narrow the claim before publication. A visual optimization also needs the applicable VHS evidence.

## Write the PR

Use a short, concrete title. Open the body with a short goal paragraph explaining the user-facing problem and intended result. For a pure refactor or dependency update, state its maintenance goal. Do not add Why/How headings. Add a brief explanation of the approach only when it helps reviewers understand behavior, a non-obvious design choice, or a tradeoff.

Use only the sections the change needs. A bug fix with a visual impact might look like:

````markdown
Keep queue navigation usable when job arguments extend beyond the terminal width. Horizontal scrolling now preserves the selected job and keeps the scrollbar aligned with the visible content.

## Reproduction

[Minimal fixture/setup, exact commands and keystrokes, relevant environment, baseline and proposed revision IDs.]

## Before / After

[Observed failure and corrected result, with the required attached screenshots or recordings.]
````

For a new visual feature, use `## Demo` with a short caption and the required media instead of manufacturing a failing baseline. Add `## Benchmarks` when required. Omit empty or inapplicable sections. If a repository template exists, preserve its required structure and comments while placing the goal paragraph first where the template permits.

Prefer connected prose. Do not list changed files, narrate the diff, paste internal plans or chat history, or add routine test plans/checklists. Include only specific verification details that materially help assess the result. Link related issues deliberately; use a closing keyword only when the PR fully resolves the issue.

## Publish and verify

Run `mise run ci` and `git diff --check`. Check that relevant regressions and UI golden coverage required by `AGENTS.md` are present. Report failed or unavailable checks accurately and resolve failures before publishing unless the user explicitly directs otherwise.

Prepare the complete title, body, and evidence before pushing. Present the full draft and inspectable local media to the user. Respect existing authorization: a request to raise a PR authorizes the focused commit, push, and PR creation; a request only to review or draft does not. Ask for approval only when it is still required by the user's instructions or execution permissions, after the result is ready to review.

Use the GitHub Code plugin for PR review, comments, and CI as required by `AGENTS.md`. Use its creation/editing tools when they support the complete body and attachments; otherwise use `gh` for publication and media upload. Write the body to a temporary file and pass `--body-file` so literal code and newlines survive.

Push only the intended branch. Look for an existing PR for the exact repository/head before creating one; update it when appropriate. Always specify the repository, base, and head on creation. For `gh` attachment syntax and recovery from partial uploads, follow [Visual evidence](references/visual-evidence.md).

After creation, attach the PR to this Codex chat with the app's `attach_artifact` tool when available. Read back the PR to verify title, full body, base/head, commits, and uploaded assets. Open the rendered PR and confirm screenshots display and recordings play. Never report completion while required attachments are missing or broken.

Report the PR URL, branch, final commit, worktree status, and any material verification limits. Retain local evidence until uploads are verified; use `trash` for temporary cleanup.
