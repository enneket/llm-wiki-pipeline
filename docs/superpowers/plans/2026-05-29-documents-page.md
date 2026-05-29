# Documents Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Documents" tab to browse, filter, and inspect fetched articles from RSS feeds.

**Architecture:** New Go handler file (`handler_documents.go`) with 3 API endpoints. Frontend adds a 6th tab with filter bar, paginated list, and detail view. Follows existing patterns from `handler_wiki.go` and `handler_status.go`.

**Tech Stack:** Go (pgx), vanilla JavaScript, Playwright E2E tests

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/web/handler_documents.go` | Create | 3 API handlers: list, detail, stats |
| `internal/web/server.go:39-51` | Modify | Register 3 new routes |
| `internal/web/static/index.html:17-18` | Modify | Add Documents tab button and section |
| `internal/web/static/app.js` | Modify | Document list/detail/filter/pagination logic |
| `internal/web/static/style.css` | Modify | Document list styles |
| `tests/frontend.spec.ts` | Modify | Documents tab E2E tests |

---

### Task 1: Backend API — handler_documents.go

**Files:**
- Create: `internal/web/handler_documents.go`
- Modify: `internal/web/server.go:39-51`

- [ ] **Step 1: Create handler_documents.go with structs and list handler**

```go
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type DocumentItem struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	FeedName  string   `json:"feed_name"`
	Tags      []string `json:"tags"`
	Source    string   `json:"source"`
	Status    string   `json:"status"`
	Summary   string   `json:"summary"`
	CreatedAt string   `json:"created_at"`
}

type DocumentDetail struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	FeedName   string   `json:"feed_name"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
	Status     string   `json:"status"`
	Content    string   `json:"content"`
	Confidence *float32 `json:"confidence,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

type DocumentListResponse struct {
	Items   []DocumentItem `json:"items"`
	Total   int            `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
}

type FeedStat struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DocumentStatsResponse struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	BySource map[string]int `json:"by_source"`
	Feeds    []FeedStat     `json:"feeds"`
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	// Build WHERE clauses
	var conditions []string
	var args []any
	argIdx := 1

	if feedID := q.Get("feed_id"); feedID != "" {
		conditions = append(conditions, fmt.Sprintf("d.feed_id = $%d", argIdx))
		args = append(args, feedID)
		argIdx++
	}
	if tag := q.Get("tag"); tag != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(d.tags)", argIdx))
		args = append(args, tag)
		argIdx++
	}
	if source := q.Get("source"); source != "" {
		conditions = append(conditions, fmt.Sprintf("d.source = $%d", argIdx))
		args = append(args, source)
		argIdx++
	}
	if status := q.Get("status"); status != "" {
		conditions = append(conditions, fmt.Sprintf("COALESCE(iq.status, 'pending') = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Sort
	sortField := "d.created_at"
	if s := q.Get("sort"); s == "title" {
		sortField = "d.title"
	}
	order := "DESC"
	if o := q.Get("order"); o == "asc" {
		order = "ASC"
	}

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM documents d
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		%s
	`, where)
	var total int
	err := s.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "failed to count documents", http.StatusInternalServerError)
		return
	}

	// Fetch page
	listQuery := fmt.Sprintf(`
		SELECT d.id, d.title, d.url, COALESCE(f.name, ''), d.tags, d.source,
		       COALESCE(iq.status, 'pending'), LEFT(d.content, 200), d.created_at::text
		FROM documents d
		LEFT JOIN feeds f ON f.id = d.feed_id
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, sortField, order, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := s.db.Pool.Query(ctx, listQuery, args...)
	if err != nil {
		http.Error(w, "failed to query documents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []DocumentItem
	for rows.Next() {
		var item DocumentItem
		if err := rows.Scan(&item.ID, &item.Title, &item.URL, &item.FeedName, &item.Tags, &item.Source, &item.Status, &item.Summary, &item.CreatedAt); err != nil {
			http.Error(w, "failed to scan document", http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DocumentListResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}
```

- [ ] **Step 2: Add detail and stats handlers to the same file**

Append to `handler_documents.go`:

```go
func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var doc DocumentDetail
	err = s.db.Pool.QueryRow(ctx, `
		SELECT d.id, d.title, d.url, COALESCE(f.name, ''), d.tags, d.source,
		       COALESCE(iq.status, 'pending'), d.content, d.confidence, d.created_at::text
		FROM documents d
		LEFT JOIN feeds f ON f.id = d.feed_id
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		WHERE d.id = $1
	`, id).Scan(&doc.ID, &doc.Title, &doc.URL, &doc.FeedName, &doc.Tags, &doc.Source, &doc.Status, &doc.Content, &doc.Confidence, &doc.CreatedAt)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleDocumentStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp DocumentStatsResponse
	resp.ByStatus = make(map[string]int)
	resp.BySource = make(map[string]int)

	// Total
	s.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM documents").Scan(&resp.Total)

	// By status
	rows, err := s.db.Pool.Query(ctx, `
		SELECT COALESCE(iq.status, 'pending'), COUNT(*)
		FROM documents d
		LEFT JOIN ingest_queue iq ON iq.document_id = d.id
		GROUP BY 1
	`)
	if err != nil {
		http.Error(w, "failed to query stats", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		resp.ByStatus[status] = count
	}

	// By source
	rows2, err := s.db.Pool.Query(ctx, `
		SELECT source, COUNT(*) FROM documents GROUP BY source
	`)
	if err != nil {
		http.Error(w, "failed to query source stats", http.StatusInternalServerError)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var source string
		var count int
		rows2.Scan(&source, &count)
		resp.BySource[source] = count
	}

	// By feed
	rows3, err := s.db.Pool.Query(ctx, `
		SELECT f.id, f.name, COUNT(d.id)
		FROM feeds f
		LEFT JOIN documents d ON d.feed_id = f.id
		GROUP BY f.id, f.name
		ORDER BY COUNT(d.id) DESC
	`)
	if err != nil {
		http.Error(w, "failed to query feed stats", http.StatusInternalServerError)
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var fs FeedStat
		rows3.Scan(&fs.ID, &fs.Name, &fs.Count)
		resp.Feeds = append(resp.Feeds, fs)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 3: Register routes in server.go**

In `internal/web/server.go`, add after line 47 (`mux.HandleFunc("GET /api/wiki/{slug}", s.handleGetWiki)`):

```go
	mux.HandleFunc("GET /api/documents", s.handleListDocuments)
	mux.HandleFunc("GET /api/documents/stats", s.handleDocumentStats)
	mux.HandleFunc("GET /api/documents/{id}", s.handleGetDocument)
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/web/handler_documents.go internal/web/server.go
git commit -m "feat: add documents API endpoints (list, detail, stats)"
```

---

### Task 2: Frontend — HTML Tab Structure

**Files:**
- Modify: `internal/web/static/index.html`

- [ ] **Step 1: Add Documents tab button**

In `index.html`, after the Wiki tab button (line 17), add:

```html
                <button class="tab" data-tab="documents">文档</button>
```

- [ ] **Step 2: Add Documents tab section**

After the Wiki tab section (after line 96 `</section>`), add:

```html
            <!-- Documents Tab -->
            <section id="documents" class="tab-content">
                <div class="doc-filters">
                    <select id="doc-filter-feed"><option value="">全部 Feed</option></select>
                    <select id="doc-filter-status">
                        <option value="">全部状态</option>
                        <option value="pending">⏳ 待处理</option>
                        <option value="processing">🔄 处理中</option>
                        <option value="done">✅ 已完成</option>
                        <option value="failed">❌ 失败</option>
                    </select>
                    <select id="doc-filter-source">
                        <option value="">全部来源</option>
                        <option value="raw">raw</option>
                        <option value="cleaned_raw">cleaned_raw</option>
                        <option value="reject">reject</option>
                    </select>
                    <input id="doc-filter-tag" placeholder="标签筛选">
                    <select id="doc-sort">
                        <option value="created_at:desc">最新优先</option>
                        <option value="created_at:asc">最早优先</option>
                        <option value="title:asc">标题 A-Z</option>
                        <option value="title:desc">标题 Z-A</option>
                    </select>
                </div>
                <div class="doc-header">
                    <span id="doc-total">共 0 篇文档</span>
                    <span id="doc-page-info">第 0/0 页</span>
                </div>
                <div id="doc-list" class="doc-list"></div>
                <div id="doc-pagination" class="doc-pagination"></div>
                <div id="doc-content" class="doc-content" style="display:none">
                    <button onclick="backToDocList()">返回列表</button>
                    <h2 id="doc-title"></h2>
                    <div class="doc-meta">
                        <span id="doc-meta-feed"></span>
                        <span id="doc-meta-status"></span>
                        <span id="doc-meta-source"></span>
                        <span id="doc-meta-date"></span>
                    </div>
                    <div id="doc-body"></div>
                </div>
            </section>
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: no errors (HTML is embedded)

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/index.html
git commit -m "feat: add documents tab HTML structure"
```

---

### Task 3: Frontend — JavaScript Logic

**Files:**
- Modify: `internal/web/static/app.js`

- [ ] **Step 1: Add tab data loading hook**

In `app.js`, in the tab click handler (around line 10-14), add after `if (tab.dataset.tab === 'settings') loadSettings();`:

```javascript
        if (tab.dataset.tab === 'documents') loadDocuments();
```

- [ ] **Step 2: Add document state and loadDocuments function**

At the end of `app.js`, add:

```javascript
// Documents
let docState = { page: 1, perPage: 20, total: 0 };

async function loadDocuments() {
    // Load stats for filter dropdowns
    try {
        const statsRes = await fetch('/api/documents/stats');
        const stats = await statsRes.json();

        const feedSelect = document.getElementById('doc-filter-feed');
        const currentFeed = feedSelect.value;
        feedSelect.innerHTML = '<option value="">全部 Feed</option>' +
            stats.feeds.filter(f => f.count > 0).map(f =>
                `<option value="${f.id}">${escapeHtml(f.name)} (${f.count})</option>`
            ).join('');
        feedSelect.value = currentFeed;
    } catch (err) {
        console.error('Failed to load doc stats:', err);
    }

    // Load documents
    await fetchDocuments();
}

async function fetchDocuments() {
    const params = new URLSearchParams();
    params.set('page', docState.page);
    params.set('per_page', docState.perPage);

    const feed = document.getElementById('doc-filter-feed').value;
    const status = document.getElementById('doc-filter-status').value;
    const source = document.getElementById('doc-filter-source').value;
    const tag = document.getElementById('doc-filter-tag').value.trim();
    const sortVal = document.getElementById('doc-sort').value;
    const [sort, order] = sortVal.split(':');

    if (feed) params.set('feed_id', feed);
    if (status) params.set('status', status);
    if (source) params.set('source', source);
    if (tag) params.set('tag', tag);
    params.set('sort', sort);
    params.set('order', order);

    try {
        const res = await fetch(`/api/documents?${params}`);
        const data = await res.json();

        docState.total = data.total;
        const totalPages = Math.max(1, Math.ceil(data.total / docState.perPage));

        document.getElementById('doc-total').textContent = `共 ${data.total} 篇文档`;
        document.getElementById('doc-page-info').textContent = `第 ${data.page}/${totalPages} 页`;

        const list = document.getElementById('doc-list');
        const items = data.items || [];
        list.innerHTML = items.map(d => {
            const statusMap = { pending: '⏳ 待处理', processing: '🔄 处理中', done: '✅ 已完成', failed: '❌ 失败' };
            return `
                <div class="doc-item">
                    <h3><a href="#" onclick="loadDocPage(${d.id}); return false;">${escapeHtml(d.title)}</a></h3>
                    <div class="doc-item-meta">
                        <span>来源: ${escapeHtml(d.feed_name)}</span>
                        ${d.tags.length ? `<span>标签: ${d.tags.map(escapeHtml).join(', ')}</span>` : ''}
                        <span>${statusMap[d.status] || d.status}</span>
                        <span>${d.created_at.split('T')[0]}</span>
                    </div>
                    <p class="doc-summary">${escapeHtml(d.summary)}</p>
                </div>
            `;
        }).join('');

        // Pagination
        const pagDiv = document.getElementById('doc-pagination');
        let pagHtml = '';
        if (data.page > 1) {
            pagHtml += `<button onclick="goDocPage(${data.page - 1})">上一页</button>`;
        }
        for (let i = 1; i <= totalPages && i <= 10; i++) {
            pagHtml += `<button onclick="goDocPage(${i})" ${i === data.page ? 'class="active"' : ''}>${i}</button>`;
        }
        if (data.page < totalPages) {
            pagHtml += `<button onclick="goDocPage(${data.page + 1})">下一页</button>`;
        }
        pagDiv.innerHTML = pagHtml;

        document.getElementById('doc-list').style.display = 'block';
        document.getElementById('doc-pagination').style.display = totalPages > 1 ? 'flex' : 'none';
        document.getElementById('doc-content').style.display = 'none';
    } catch (err) {
        console.error('Failed to load documents:', err);
    }
}

function goDocPage(page) {
    docState.page = page;
    fetchDocuments();
}

async function loadDocPage(id) {
    try {
        const res = await fetch(`/api/documents/${id}`);
        if (!res.ok) throw new Error('Not found');
        const doc = await res.json();

        document.getElementById('doc-title').textContent = doc.title;
        document.getElementById('doc-meta-feed').textContent = '来源: ' + doc.feed_name;
        const statusMap = { pending: '⏳ 待处理', processing: '🔄 处理中', done: '✅ 已完成', failed: '❌ 失败' };
        document.getElementById('doc-meta-status').textContent = statusMap[doc.status] || doc.status;
        document.getElementById('doc-meta-source').textContent = '类型: ' + doc.source;
        document.getElementById('doc-meta-date').textContent = doc.created_at.split('T')[0];
        document.getElementById('doc-body').innerHTML = renderMarkdown(doc.content);

        document.getElementById('doc-list').style.display = 'none';
        document.getElementById('doc-pagination').style.display = 'none';
        document.getElementById('doc-content').style.display = 'block';
    } catch (err) {
        alert('加载失败: ' + err.message);
    }
}

function backToDocList() {
    document.getElementById('doc-list').style.display = 'block';
    document.getElementById('doc-pagination').style.display = 'flex';
    document.getElementById('doc-content').style.display = 'none';
}
```

- [ ] **Step 3: Add filter event listeners**

After the document functions, add:

```javascript
// Document filter listeners
['doc-filter-feed', 'doc-filter-status', 'doc-filter-source', 'doc-sort'].forEach(id => {
    document.getElementById(id).addEventListener('change', () => {
        docState.page = 1;
        fetchDocuments();
    });
});
document.getElementById('doc-filter-tag').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
        docState.page = 1;
        fetchDocuments();
    }
});
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/app.js
git commit -m "feat: add documents tab JavaScript logic"
```

---

### Task 4: Frontend — CSS Styles

**Files:**
- Modify: `internal/web/static/style.css`

- [ ] **Step 1: Add document list styles**

At the end of `style.css`, add:

```css
/* Documents */
.doc-filters {
    display: flex;
    gap: 10px;
    margin-bottom: 15px;
    flex-wrap: wrap;
}

.doc-filters select,
.doc-filters input {
    padding: 8px 12px;
    border: 1px solid #ddd;
    border-radius: 4px;
    font-size: 14px;
}

.doc-filters input {
    min-width: 120px;
}

.doc-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 15px;
    color: #666;
    font-size: 14px;
}

.doc-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.doc-item {
    padding: 15px;
    background: #f9f9f9;
    border-radius: 4px;
}

.doc-item h3 {
    margin-bottom: 5px;
}

.doc-item h3 a {
    color: #2c3e50;
    text-decoration: none;
}

.doc-item h3 a:hover {
    color: #3498db;
}

.doc-item-meta {
    display: flex;
    gap: 15px;
    flex-wrap: wrap;
    font-size: 13px;
    color: #666;
    margin-bottom: 8px;
}

.doc-summary {
    font-size: 14px;
    color: #555;
    line-height: 1.5;
}

.doc-pagination {
    display: flex;
    gap: 5px;
    justify-content: center;
    margin-top: 20px;
    flex-wrap: wrap;
}

.doc-pagination button {
    padding: 6px 12px;
    font-size: 14px;
}

.doc-pagination button.active {
    background: #2c3e50;
}

.doc-content {
    line-height: 1.8;
}

.doc-content button {
    margin-bottom: 20px;
}

.doc-content h2 {
    margin-bottom: 10px;
    color: #2c3e50;
}

.doc-meta {
    display: flex;
    gap: 15px;
    flex-wrap: wrap;
    font-size: 13px;
    color: #666;
    margin-bottom: 20px;
    padding-bottom: 15px;
    border-bottom: 1px solid #eee;
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/style.css
git commit -m "feat: add documents tab CSS styles"
```

---

### Task 5: E2E Tests

**Files:**
- Modify: `tests/frontend.spec.ts`

- [ ] **Step 1: Add mock helpers**

In `frontend.spec.ts`, after the existing mock helpers, add:

```typescript
async function mockDocuments(page: Page, items = [
  { id: 1, title: 'AI News', url: 'https://example.com/1', feed_name: 'Hacker News', tags: ['AI'], source: 'cleaned_raw', status: 'done', summary: 'An article about AI...', created_at: '2026-05-29T10:00:00Z' },
  { id: 2, title: 'Go 1.25 Released', url: 'https://example.com/2', feed_name: 'Go Blog', tags: ['go'], source: 'raw', status: 'pending', summary: 'Go 1.25 release notes...', created_at: '2026-05-28T08:00:00Z' },
], total = 2) {
  await page.route('**/api/documents?**', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items, total, page: 1, per_page: 20 }) })
  );
  await page.route('**/api/documents/stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
      total, by_status: { done: 1, pending: 1 }, by_source: { cleaned_raw: 1, raw: 1 },
      feeds: [{ id: 1, name: 'Hacker News', count: 1 }, { id: 2, name: 'Go Blog', count: 1 }],
    }) })
  );
}

async function mockDocDetail(page: Page, doc = { id: 1, title: 'AI News', url: 'https://example.com/1', feed_name: 'Hacker News', tags: ['AI'], source: 'cleaned_raw', status: 'done', content: '# AI News\n\n**Big** developments.', confidence: 0.95, created_at: '2026-05-29T10:00:00Z' }) {
  await page.route(`**/api/documents/${doc.id}`, route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(doc) })
  );
}
```

- [ ] **Step 2: Add Documents tab test suite**

At the end of the test file, add:

```typescript
// ─── Documents Tab ───

test.describe('Documents Tab', () => {
  test('tab click loads documents', async ({ page }) => {
    await mockDocuments(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await expect(page.locator('.tab[data-tab="documents"]')).toHaveClass(/active/);

    const items = page.locator('.doc-item');
    await expect(items).toHaveCount(2);
    await expect(items.first().locator('h3 a')).toHaveText('AI News');
  });

  test('document item shows metadata', async ({ page }) => {
    await mockDocuments(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    const meta = page.locator('.doc-item-meta').first();
    await expect(meta).toContainText('Hacker News');
    await expect(meta).toContainText('AI');
    await expect(meta).toContainText('已完成');
  });

  test('click document shows detail view', async ({ page }) => {
    await mockDocuments(page);
    await mockDocDetail(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await page.locator('.doc-item h3 a').first().click();

    await expect(page.locator('#doc-content')).toBeVisible();
    await expect(page.locator('#doc-list')).not.toBeVisible();
    await expect(page.locator('#doc-title')).toHaveText('AI News');
  });

  test('back button returns to list', async ({ page }) => {
    await mockDocuments(page);
    await mockDocDetail(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await page.locator('.doc-item h3 a').first().click();
    await expect(page.locator('#doc-content')).toBeVisible();

    await page.click('button:text("返回列表")');
    await expect(page.locator('#doc-list')).toBeVisible();
    await expect(page.locator('#doc-content')).not.toBeVisible();
  });

  test('filter dropdowns are populated', async ({ page }) => {
    await mockDocuments(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');

    const feedSelect = page.locator('#doc-filter-feed');
    await expect(feedSelect.locator('option')).toHaveCount(3); // "全部" + 2 feeds
  });

  test('status filter triggers reload', async ({ page }) => {
    let requestUrl = '';
    await page.route('**/api/documents/stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ total: 0, by_status: {}, by_source: {}, feeds: [] }) })
    );
    await page.route('**/api/documents?**', route => {
      requestUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], total: 0, page: 1, per_page: 20 }) });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await page.selectOption('#doc-filter-status', 'done');

    expect(requestUrl).toContain('status=done');
  });

  test('pagination shows page info', async ({ page }) => {
    await mockDocuments(page, [], 50);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await expect(page.locator('#doc-total')).toHaveText('共 50 篇文档');
  });
});
```

- [ ] **Step 3: Run tests**

Run: `cd tests && npx playwright test --reporter=list`
Expected: all tests pass (including new Documents tests)

- [ ] **Step 4: Commit**

```bash
git add tests/frontend.spec.ts
git commit -m "test: add documents tab E2E tests"
```

---

### Task 6: Build, Deploy, Verify

- [ ] **Step 1: Build Docker image**

Run: `docker compose up -d --build`
Expected: image built, container started

- [ ] **Step 2: Run all tests**

Run: `cd tests && npx playwright test --reporter=list`
Expected: all tests pass

- [ ] **Step 3: Manual verification**

Open http://localhost:6006/, click "文档" tab, verify:
- Filter dropdowns populated
- Document list renders
- Click document shows detail
- Back button works
- Pagination works

- [ ] **Step 4: Final commit and push**

```bash
git push
```
