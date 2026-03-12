#!/usr/bin/env bash
# teams-notify.sh — Send notifications to Microsoft Teams via Graph API
#
# Usage: teams-notify.sh <event_type> <message>
#
# Event types: work_complete | pr_opened | pr_merged | issue_closed | decisions
#
# Reads channel config from .squad/identity/teams-config.json relative to TEAM_ROOT.
# Uses `az rest` to POST messages to the configured Teams channel.
#
# Exit behavior: Always exits 0 to never break the caller's workflow.
# Errors are logged to stderr as warnings.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEAM_ROOT="${TEAM_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
CONFIG_FILE="$TEAM_ROOT/.squad/identity/teams-config.json"

# --- Help -------------------------------------------------------------------

show_help() {
    cat <<'EOF'
teams-notify.sh — Send notifications to a Microsoft Teams channel

USAGE:
    teams-notify.sh <event_type> <message>
    teams-notify.sh --help

EVENT TYPES:
    work_complete   Squad member finished a task
    pr_opened       Pull request was opened
    pr_merged       Pull request was merged
    issue_closed    GitHub issue was closed
    decisions       Team decision was recorded

ARGUMENTS:
    event_type      One of the event types listed above
    message         The notification message body (string)

ENVIRONMENT:
    TEAM_ROOT       Root of the team directory (default: auto-detected)

CONFIG:
    Reads .squad/identity/teams-config.json for channel details and
    per-event enable/disable flags.

EXAMPLES:
    teams-notify.sh work_complete "Linus finished PR #242"
    teams-notify.sh pr_opened "PR #300: Add grader weighting"
    teams-notify.sh decisions "Adopted bootstrap CI for statistical analysis"

EXIT BEHAVIOR:
    Always exits 0. Failures are logged to stderr as warnings.
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    show_help
    exit 0
fi

# --- Argument validation -----------------------------------------------------

if [[ $# -lt 2 ]]; then
    echo "Warning: teams-notify.sh requires 2 arguments: <event_type> <message>" >&2
    echo "Run with --help for usage." >&2
    exit 0
fi

EVENT_TYPE="$1"
MESSAGE="$2"

VALID_EVENTS="work_complete pr_opened pr_merged issue_closed decisions"
if ! echo "$VALID_EVENTS" | grep -qw "$EVENT_TYPE"; then
    echo "Warning: Unknown event type '$EVENT_TYPE'. Valid: $VALID_EVENTS" >&2
    exit 0
fi

# --- Config reading -----------------------------------------------------------

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Warning: Config not found at $CONFIG_FILE" >&2
    exit 0
fi

# Prefer jq, fall back to grep/sed
if command -v jq &>/dev/null; then
    USE_JQ=true
else
    USE_JQ=false
    echo "Info: jq not found, using grep/sed fallback for JSON parsing" >&2
fi

read_config_value() {
    local key="$1"
    if $USE_JQ; then
        jq -r "$key" "$CONFIG_FILE" 2>/dev/null
    else
        # Fallback: simple grep/sed for flat keys
        # Works for top-level string/boolean values
        local simple_key
        simple_key=$(echo "$key" | sed 's/^\.//; s/\./_/g')
        grep -o "\"${simple_key}\"[[:space:]]*:[[:space:]]*[^,}]*" "$CONFIG_FILE" \
            | sed 's/.*:[[:space:]]*//; s/"//g; s/[[:space:]]*$//'
    fi
}

read_notify_flag() {
    local event="$1"
    if $USE_JQ; then
        jq -r ".notify_on.${event} // false" "$CONFIG_FILE" 2>/dev/null
    else
        # Fallback: look for the key inside notify_on block
        grep -o "\"${event}\"[[:space:]]*:[[:space:]]*[a-z]*" "$CONFIG_FILE" \
            | sed 's/.*:[[:space:]]*//' | head -1
    fi
}

# --- Check enabled ------------------------------------------------------------

ENABLED=$(read_config_value '.enabled')
if [[ "$ENABLED" != "true" ]]; then
    # Notifications disabled globally — exit silently
    exit 0
fi

# --- Check az CLI availability ------------------------------------------------

if ! command -v az &>/dev/null; then
    # az CLI not installed — exit silently
    exit 0
fi

# Check if logged in (fast check via account show)
if ! az account show &>/dev/null 2>&1; then
    # Not logged in — exit silently
    exit 0
fi

# --- Check event is enabled ---------------------------------------------------

EVENT_ENABLED=$(read_notify_flag "$EVENT_TYPE")
if [[ "$EVENT_ENABLED" != "true" ]]; then
    # This event type is disabled — exit silently
    exit 0
fi

# --- Read channel details -----------------------------------------------------

GROUP_ID=$(read_config_value '.group_id')
CHANNEL_ID=$(read_config_value '.channel_id')

if [[ -z "$GROUP_ID" || -z "$CHANNEL_ID" ]]; then
    echo "Warning: Missing group_id or channel_id in config" >&2
    exit 0
fi

# --- Format message -----------------------------------------------------------

case "$EVENT_TYPE" in
    work_complete)
        HEADER="🏗️ Squad Work Complete"
        ;;
    pr_opened)
        HEADER="📋 Pull Request Opened"
        ;;
    pr_merged)
        HEADER="✅ Pull Request Merged"
        ;;
    issue_closed)
        HEADER="🎯 Issue Closed"
        ;;
    decisions)
        HEADER="📌 Team Decision"
        ;;
esac

# Escape HTML special chars in message to prevent injection
escape_html() {
    local text="$1"
    text="${text//&/&amp;}"
    text="${text//</&lt;}"
    text="${text//>/&gt;}"
    text="${text//\"/&quot;}"
    echo "$text"
}

SAFE_MESSAGE=$(escape_html "$MESSAGE")

HTML_CONTENT="<h3>${HEADER}</h3><p>${SAFE_MESSAGE}</p><hr/><p style=\"color:gray;font-size:small;\">Sent by Waza Squad</p>"

# --- Build request body -------------------------------------------------------

# Use jq to build JSON if available (handles escaping properly)
if $USE_JQ; then
    REQUEST_BODY=$(jq -n --arg content "$HTML_CONTENT" '{body: {contentType: "html", content: $content}}')
else
    # Manual JSON construction — escape embedded quotes
    ESCAPED_CONTENT="${HTML_CONTENT//\"/\\\"}"
    REQUEST_BODY="{\"body\":{\"contentType\":\"html\",\"content\":\"${ESCAPED_CONTENT}\"}}"
fi

# --- Send notification --------------------------------------------------------

API_URL="https://graph.microsoft.com/v1.0/teams/${GROUP_ID}/channels/${CHANNEL_ID}/messages"

if ! az rest \
    --method POST \
    --url "$API_URL" \
    --headers "Content-Type=application/json" \
    --body "$REQUEST_BODY" \
    &>/dev/null 2>&1; then
    echo "Warning: Failed to send Teams notification for event '$EVENT_TYPE'" >&2
    # Exit 0 — never break the caller
    exit 0
fi

echo "Teams notification sent: $EVENT_TYPE" >&2
exit 0
