# Design: Progress Persistence on Page Refresh

**Date**: 2026-06-04
**Status**: Draft
**Author**: opencode

## Problem

When users click "Fetch Feeds" or "LLM Process" buttons, the progress is tracked in memory and displayed via polling. However, if the user refreshes the page, the polling interval is cleared and the progress bar disappears, even though the backend task is still running.

## Requirements

| Requirement | Description |
|-------------|-------------|
| Scope | Both Feed fetching and LLM processing tasks |
| Recovery | Auto-detect and restore progress bar on page load |
| Completion | Auto-hide progress bar when task completes |

## Solution: Frontend Auto-Polling on Load

### Approach

Keep the existing in-memory state on the backend. Add frontend logic to check task status on page load and restore the progress bar if a task is running.

### Changes Required

**File**: `internal/web/static/app.js`

**New Function**: `checkAndRestoreProgress()`

```javascript
async function checkAndRestoreProgress() {
    // Check Feed fetch status
    try {
        const fetchRes = await fetch('/api/feeds/fetch/status');
        const fetchData = await fetchRes.json();
        if (fetchData.running) {
            startFetchProgress();
        }
    } catch (err) {
        console.error('Failed to check fetch status:', err);
    }
    
    // Check LLM process status
    try {
        const processRes = await fetch('/api/documents/process/status');
        const processData = await processRes.json();
        if (processData.running) {
            startProcessProgress();
        }
    } catch (err) {
        console.error('Failed to check process status:', err);
    }
}
```

**Modification**: Add call to `checkAndRestoreProgress()` in `DOMContentLoaded` handler.

### No Backend Changes

The existing APIs already provide all required information:

- `GET /api/feeds/fetch/status` → `{ running, total, completed, current }`
- `GET /api/documents/process/status` → `{ running, total, completed, current }`

### User Flow

1. User clicks "Fetch Feeds" or "LLM Process"
2. Progress bar appears and polling begins
3. User refreshes the page
4. On page load, `checkAndRestoreProgress()` is called
5. If `running: true`, progress bar is restored and polling resumes
6. When task completes, progress bar auto-hides

### Edge Cases

| Case | Behavior |
|------|----------|
| Server restart | Task is interrupted, progress not restored (acceptable) |
| Multiple tabs | Each tab independently polls and shows progress |
| Network error during check | Progress not shown, user can manually start new task |

## Testing

1. Start a task (Feed fetch or LLM process)
2. Refresh the page while task is running
3. Verify progress bar appears and shows current progress
4. Wait for task to complete
5. Verify progress bar auto-hides
