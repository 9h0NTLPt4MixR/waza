#!/usr/bin/env bash
# ralph-watch.sh — Local watchdog that polls GitHub for repo activity
# and posts noteworthy events to Teams via teams-notify.sh.
#
# This is the "Option 1" local process that runs on Shayne's machine.
# NOT GitHub Actions — Shayne explicitly said "do not update the gh
# actions heartbeat, it does nothing for us."
#
# Usage:
#   .squad/scripts/ralph-watch.sh                  # polls every 10 minutes
#   .squad/scripts/ralph-watch.sh --interval 5     # polls every 5 minutes
#   .squad/scripts/ralph-watch.sh --interval 30    # polls every 30 minutes
#
# Requires: gh (authenticated), jq, teams-notify.sh
# State: .squad/identity/.ralph-watch-state.json

set -euo pipefail

# ---------------------------------------------------------------------------
# Paths & defaults
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEAM_ROOT="${TEAM_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
NOTIFY_SCRIPT="$SCRIPT_DIR/teams-notify.sh"
STATE_FILE="$TEAM_ROOT/.squad/identity/.ralph-watch-state.json"
INTERVAL_MINUTES=10
CYCLE=0

# Counters for shutdown summary
TOTAL_PR_MERGED=0
TOTAL_ISSUE_CLOSED=0
TOTAL_COMMIT_BATCHES=0

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case "$1" in
        --interval)
            INTERVAL_MINUTES="${2:?'--interval requires a number (minutes)'}"
            shift 2
            ;;
        --help|-h)
            cat <<'EOF'
ralph-watch.sh — Local watchdog for GitHub activity → Teams notifications

USAGE:
    ralph-watch.sh                    # polls every 10 minutes (default)
    ralph-watch.sh --interval 5      # polls every 5 minutes
    ralph-watch.sh --interval 30     # polls every 30 minutes

WHAT IT WATCHES:
    • Recently merged PRs
    • Recently closed issues
    • New commits on main

Each new event is posted to the Teams channel via teams-notify.sh.
State is tracked in .squad/identity/.ralph-watch-state.json so only
genuinely new activity triggers notifications.

Press Ctrl+C for graceful shutdown with a summary of reported events.

REQUIRES:
    gh    — GitHub CLI, authenticated
    jq    — JSON processor
    teams-notify.sh — in the same scripts directory
EOF
            exit 0
            ;;
        *)
            echo "Unknown argument: $1 (try --help)" >&2
            exit 1
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
preflight() {
    local ok=true

    if ! command -v gh &>/dev/null; then
        echo "❌ gh CLI not found. Install: https://cli.github.com" >&2
        ok=false
    elif ! gh auth status &>/dev/null 2>&1; then
        echo "❌ gh CLI not authenticated. Run: gh auth login" >&2
        ok=false
    fi

    if ! command -v jq &>/dev/null; then
        echo "❌ jq not found. Install: brew install jq" >&2
        ok=false
    fi

    if [[ ! -x "$NOTIFY_SCRIPT" ]]; then
        echo "❌ teams-notify.sh not found or not executable at: $NOTIFY_SCRIPT" >&2
        ok=false
    fi

    if [[ "$ok" != "true" ]]; then
        echo "Pre-flight checks failed. Aborting." >&2
        exit 1
    fi

    echo "✅ Pre-flight OK (gh authenticated, jq available, teams-notify.sh ready)"
}

# ---------------------------------------------------------------------------
# State management
# ---------------------------------------------------------------------------
# State schema:
# {
#   "last_check_timestamp": "2026-03-12T10:00:00Z",
#   "last_commit_sha": "abc123...",
#   "known_merged_prs": [42, 43],
#   "known_closed_issues": [10, 11]
# }

init_state() {
    # First run: snapshot current state so we don't notify about
    # pre-existing activity.
    echo "🔧 First run — initializing state (no notifications for existing activity)"

    local now
    now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

    # Grab current merged PRs
    local merged_prs
    merged_prs=$(gh pr list --state merged --json number --limit 50 2>/dev/null \
        | jq '[.[].number]' 2>/dev/null || echo '[]')

    # Grab current closed issues
    local closed_issues
    closed_issues=$(gh issue list --state closed --json number --limit 50 2>/dev/null \
        | jq '[.[].number]' 2>/dev/null || echo '[]')

    # Grab HEAD of main
    local head_sha
    head_sha=$(gh api repos/:owner/:repo/commits/main --jq '.sha' 2>/dev/null || echo "")

    jq -n \
        --arg ts "$now" \
        --arg sha "$head_sha" \
        --argjson prs "$merged_prs" \
        --argjson issues "$closed_issues" \
        '{
            last_check_timestamp: $ts,
            last_commit_sha: $sha,
            known_merged_prs: $prs,
            known_closed_issues: $issues
        }' > "$STATE_FILE"

    echo "   State saved to $STATE_FILE"
}

load_state() {
    if [[ ! -f "$STATE_FILE" ]]; then
        init_state
    fi
    # Validate it's parseable JSON
    if ! jq empty "$STATE_FILE" 2>/dev/null; then
        echo "⚠️  Corrupted state file — reinitializing" >&2
        init_state
    fi
}

read_state_field() {
    jq -r "$1" "$STATE_FILE" 2>/dev/null || echo ""
}

save_state() {
    local now="$1"
    local new_sha="$2"
    local merged_prs="$3"
    local closed_issues="$4"

    jq \
        --arg ts "$now" \
        --arg sha "$new_sha" \
        --argjson prs "$merged_prs" \
        --argjson issues "$closed_issues" \
        '.last_check_timestamp = $ts
         | .last_commit_sha = $sha
         | .known_merged_prs = (.known_merged_prs + $prs | unique)
         | .known_closed_issues = (.known_closed_issues + $issues | unique)' \
        "$STATE_FILE" > "${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE"
}

# ---------------------------------------------------------------------------
# Activity checks
# ---------------------------------------------------------------------------

check_merged_prs() {
    local known_prs
    known_prs=$(jq -c '.known_merged_prs // []' "$STATE_FILE")

    local merged
    merged=$(gh pr list --state merged --json number,title,author --limit 20 2>/dev/null || echo '[]')

    # Filter to PRs not in known list
    local new_prs
    new_prs=$(echo "$merged" | jq -c --argjson known "$known_prs" \
        '[.[] | select(.number as $n | $known | index($n) | not)]')

    local count
    count=$(echo "$new_prs" | jq 'length')

    if [[ "$count" -gt 0 ]]; then
        while read -r pr; do
            local num title author
            num=$(echo "$pr" | jq -r '.number')
            title=$(echo "$pr" | jq -r '.title')
            author=$(echo "$pr" | jq -r '.author.login // .author.name // "unknown"')

            echo "   📦 PR #${num} merged: ${title} (by ${author})"
            "$NOTIFY_SCRIPT" pr_merged "PR #${num} merged: ${title} (by ${author})" 2>/dev/null || true
            TOTAL_PR_MERGED=$((TOTAL_PR_MERGED + 1))
        done < <(echo "$new_prs" | jq -c '.[]')
    fi

    # Return the new PR numbers for state update
    echo "$new_prs" | jq -c '[.[].number]'
}

check_closed_issues() {
    local known_issues
    known_issues=$(jq -c '.known_closed_issues // []' "$STATE_FILE")

    local closed
    closed=$(gh issue list --state closed --json number,title --limit 20 2>/dev/null || echo '[]')

    # Filter to issues not in known list
    local new_issues
    new_issues=$(echo "$closed" | jq -c --argjson known "$known_issues" \
        '[.[] | select(.number as $n | $known | index($n) | not)]')

    local count
    count=$(echo "$new_issues" | jq 'length')

    if [[ "$count" -gt 0 ]]; then
        while read -r issue; do
            local num title
            num=$(echo "$issue" | jq -r '.number')
            title=$(echo "$issue" | jq -r '.title')

            echo "   ✅ Issue #${num} closed: ${title}"
            "$NOTIFY_SCRIPT" issue_closed "Issue #${num} closed: ${title}" 2>/dev/null || true
            TOTAL_ISSUE_CLOSED=$((TOTAL_ISSUE_CLOSED + 1))
        done < <(echo "$new_issues" | jq -c '.[]')
    fi

    echo "$new_issues" | jq -c '[.[].number]'
}

check_new_commits() {
    local last_sha
    last_sha=$(read_state_field '.last_commit_sha')

    local current_sha
    current_sha=$(gh api repos/:owner/:repo/commits/main --jq '.sha' 2>/dev/null || echo "")

    if [[ -z "$current_sha" ]]; then
        echo "$last_sha"
        return
    fi

    if [[ "$current_sha" == "$last_sha" ]]; then
        echo "$current_sha"
        return
    fi

    # Count new commits and gather authors
    if [[ -n "$last_sha" ]]; then
        local compare
        compare=$(gh api "repos/:owner/:repo/compare/${last_sha}...${current_sha}" \
            --jq '{ahead_by: .ahead_by, authors: [.commits[].author.login // .commits[].commit.author.name] | unique}' \
            2>/dev/null || echo '{}')

        local ahead authors_str
        ahead=$(echo "$compare" | jq -r '.ahead_by // 0')
        authors_str=$(echo "$compare" | jq -r '(.authors // []) | join(", ")')

        if [[ "$ahead" -gt 0 && -n "$authors_str" ]]; then
            echo "   🔨 ${ahead} new commit(s) on main by: ${authors_str}"
            "$NOTIFY_SCRIPT" work_complete "${ahead} new commit(s) pushed to main by: ${authors_str}" 2>/dev/null || true
            TOTAL_COMMIT_BATCHES=$((TOTAL_COMMIT_BATCHES + 1))
        fi
    fi

    echo "$current_sha"
}

# ---------------------------------------------------------------------------
# Poll cycle
# ---------------------------------------------------------------------------

poll_cycle() {
    CYCLE=$((CYCLE + 1))
    local now
    now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    local display_time
    display_time="$(date +"%H:%M")"

    echo "🔄 Ralph watching... (cycle ${CYCLE}, last check: ${display_time})"

    # Run checks, capturing new items for state update
    local new_pr_numbers new_issue_numbers new_sha

    # Merged PRs — capture output, but the function also prints + notifies
    new_pr_numbers=$(check_merged_prs)

    # Closed issues
    new_issue_numbers=$(check_closed_issues)

    # New commits (returns the new HEAD sha)
    new_sha=$(check_new_commits)

    # Update state
    save_state "$now" "$new_sha" "$new_pr_numbers" "$new_issue_numbers"
}

# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------

shutdown_handler() {
    echo ""
    echo "🛑 Ralph shutting down (ran ${CYCLE} cycles)"
    echo "   📊 Summary:"
    echo "      PRs merged notified:    ${TOTAL_PR_MERGED}"
    echo "      Issues closed notified:  ${TOTAL_ISSUE_CLOSED}"
    echo "      Commit batch notified:   ${TOTAL_COMMIT_BATCHES}"
    echo "   👋 See you next time!"
    exit 0
}

trap shutdown_handler SIGINT SIGTERM

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------

main() {
    echo "🐺 Ralph Watch — local GitHub activity watchdog"
    echo "   Interval: every ${INTERVAL_MINUTES} minutes"
    echo "   State:    ${STATE_FILE}"
    echo "   Notify:   ${NOTIFY_SCRIPT}"
    echo ""

    preflight
    load_state

    echo ""
    echo "🟢 Watching started. Press Ctrl+C to stop."
    echo ""

    while true; do
        poll_cycle
        echo "   💤 Sleeping ${INTERVAL_MINUTES} minutes..."
        echo ""
        sleep "$((INTERVAL_MINUTES * 60))"
    done
}

main
