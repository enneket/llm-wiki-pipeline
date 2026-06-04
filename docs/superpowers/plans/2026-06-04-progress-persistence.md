# Progress Persistence on Page Refresh - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add frontend logic to automatically detect and restore progress bars for Feed fetching and LLM processing tasks when the page is refreshed.

**Architecture:** Modify only the frontend JavaScript to check task status on page load. If a task is running, restore the progress bar and resume polling. No backend changes required.

**Tech Stack:** Vanilla JavaScript, Fetch API

---

## File Structure

| File | Responsibility |
|------|----------------|
| `internal/web/static/app.js` | Add `checkAndRestoreProgress()` function and call it on `DOMContentLoaded` |

---

### Task 1: Add Progress Check Function

**Files:**
- Modify: `internal/web/static/app.js:46-49`

- [ ] **Step 1: Add the checkAndRestoreProgress function**

Add the following function after the existing `stopProcessProgress()` function (around line 765):

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

- [ ] **Step 2: Call the function on page load**

Modify the `DOMContentLoaded` event listener (around line 46) to include the new function call:

```javascript
// Initial tab load
document.addEventListener('DOMContentLoaded', () => {
    const tabName = window.location.hash.slice(1) || 'query';
    switchTab(tabName);
    
    // Check and restore progress for running tasks
    checkAndRestoreProgress();
});
```

- [ ] **Step 3: Verify the changes**

Open the application in a browser:
1. Start a Feed fetch or LLM process task
2. Refresh the page while the task is running
3. Verify the progress bar appears and continues updating
4. Wait for the task to complete
5. Verify the progress bar auto-hides

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/app.js
git commit -m "feat: restore progress bars on page refresh"
```
