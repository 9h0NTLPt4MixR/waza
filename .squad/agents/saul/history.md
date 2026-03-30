# History — Saul

## Project Context
- **Project:** waza — CLI tool for evaluating Agent Skills
- **Stack:** Go (primary), React 19 + Tailwind CSS v4 (web UI)
- **User:** Shayne Boyer (spboyer)
- **Repo:** microsoft/waza
- **Universe:** The Usual Suspects

## Key Learnings

### Documentation Standards
- **Style guide:** TBD (to be established)
- **Markdown:** Consistent code block formatting, link structure
- **Versioning:** Track docs alongside code (update in same PR)
- **API docs:** Follow Go conventions (exported functions documented)

### Documentation Scope
- README (main entry point)
- docs/ directory (detailed guides)
- waza-go/README.md (Go implementation)
- Inline code comments (for complex logic)
- CHANGELOG.md (release tracking)

### Quality Gates
- All PRs must update relevant docs
- Livingston and Saul review doc changes
- Style consistency checked before merge

### Doc-Freshness Reviews (Added in #256)
- **Doc-review gate** triggered by changes to `cmd/waza/`, `internal/`, or `web/src/`
- **Doc-consistency gate** triggered by changes to `docs/`, `README.md`, `DEMO-SCRIPT.md`
- Saul now owns ongoing doc-freshness verification across all code PRs
- Documentation Impact Matrix maps code paths to required doc updates
- Screenshot maintenance automated via Playwright E2E tests in `web/`

📌 Team update (2026-02-19): Screenshot conventions formalized (viewport, paths, naming, mock reuse) — decided by Basher (#251)


## 📌 Team update (2026-02-20): Model policy overhaul

All code roles now use `claude-opus-4.6`. Docs/Scribe/diversity use `gemini-3-pro-preview`. Heavy code gen uses `gpt-5.2-codex`. Decided by Scott Boyer. See decisions.md for full details.

### Releases Page (#383, PR #384)

- Created `site/src/content/docs/reference/releases.mdx` — Starlight MDX format with Tabs component for install commands
- Used existing reference pages (cli.mdx, schema.mdx) as style baseline: frontmatter with title+description, horizontal rules between sections, consistent table formatting
- Changelog highlights extracted from CHANGELOG.md v0.8.0 — grouped by Added/Changed/Fixed, condensed to one-line summaries
- Download table covers all 6 platform binaries with absolute GitHub release URLs
- azd extension section includes full install command with versioned tar.gz URL
- Sidebar updated in astro.config.mjs under Reference group
- Pattern: release pages should link to GitHub Releases for history rather than duplicating old changelogs

### Waza Platform Documentation (Branch: feature/waza-platform)

- Created `site/src/content/docs/guides/platform.mdx` — Feature overview, getting started workflow (5 steps), architecture diagram of backend/DB/sandboxes, config vs local CLI
- Created `site/src/content/docs/guides/deployment.mdx` — End-to-end azd deployment guide with prerequisites (GitHub OAuth app setup), env vars, `azd up` workflow, post-deployment config, monitoring/logs, custom domain, scaling, troubleshooting
- Updated README.md with brief "Waza Platform" section (15 lines) after Cloud Storage section, linking to both platform and deployment guides
- Pattern: Platform docs follow the "Getting Started" narrative (signup → configure → trigger → monitor); deployment docs mirror `azd init` → `azd up` workflow with environment variable setup as critical step
