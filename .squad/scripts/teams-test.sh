#!/usr/bin/env bash
# teams-test.sh — Send a test notification to verify Teams integration
#
# Usage: teams-test.sh [custom_message]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NOTIFY_SCRIPT="$SCRIPT_DIR/teams-notify.sh"

if [[ ! -x "$NOTIFY_SCRIPT" ]]; then
    echo "Error: teams-notify.sh not found or not executable at $NOTIFY_SCRIPT" >&2
    exit 1
fi

MESSAGE="${1:-🧪 Waza Squad notifications are working!}"

echo "Sending test notification to Teams..."
"$NOTIFY_SCRIPT" work_complete "$MESSAGE"

echo "Done. Check the Waza Squad channel in Teams."
