# Documents Page Design Spec

## Context

The system fetches articles from RSS feeds but has no UI to browse them. Users can only see Wiki pages (processed output) but not the raw fetched documents. A new "Documents" tab is needed to browse, filter, and inspect fetched articles.

## Approach

Add a new "Documents" tab to the existing SPA, with 3 new API endpoints and full CRUD browsing UI.

## Backend API

### Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/documents` | Document list (paginated + filtered + sorted) |
| `GET` | `/api/documents/{id}` | Single document detail |
| `GET` | `/api/documents/stats` | Document stats for filter counts |

### `GET /api/documents` Query Parameters

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `page` | int | 1 | Page number |
| `per_page` | int | 20 | Items per page |
| `feed_id` | int | - | Filter by feed ID |
| `tag` | string | - | Filter by tag |
| `source` | string | - | Filter by source type (raw/cleaned_raw/reject) |
| `status` | string | - | Filter by ingest status (pending/processing/done/failed) |
| `sort` | string | created_at | Sort field (created_at/title) |
| `order` | string | desc | Sort direction (asc/desc) |

### Response Format

```json
{
  "items": [
    {
      "id": 1,
      "title": "Article Title",
      "url": "https://...",
      "feed_name": "Hacker News",
      "tags": ["tech", "AI"],
      "source": "cleaned_raw",
      "status": "done",
      "summary": "First 200 chars of content...",
      "created_at": "2026-05-29T10:00:00Z"
    }
  ],
  "total": 150,
  "page": 1,
  "per_page": 20
}
```

### `GET /api/documents/{id}` Response

```json
{
  "id": 1,
  "title": "Article Title",
  "url": "https://...",
  "feed_name": "Hacker News",
  "tags": ["tech", "AI"],
  "source": "cleaned_raw",
  "status": "done",
  "content": "Full article content...",
  "confidence": 0.95,
  "created_at": "2026-05-29T10:00:00Z"
}
```

### `GET /api/documents/stats` Response

```json
{
  "total": 150,
  "by_status": { "pending": 5, "processing": 2, "done": 140, "failed": 3 },
  "by_source": { "raw": 20, "cleaned_raw": 125, "reject": 5 },
  "feeds": [
    { "id": 1, "name": "Hacker News", "count": 50 },
    { "id": 2, "name": "Go Blog", "count": 30 }
  ]
}
```

### Database Queries

- List: `documents` LEFT JOIN `ingest_queue` (for status) LEFT JOIN `feeds` (for feed name)
- Detail: Single document query with full content
- Stats: Aggregate queries for counts by status, source, feed

## Frontend UI

### Tab

New "文档" tab added after "Wiki 浏览" in the nav bar.

### Layout

```
┌─────────────────────────────────────────────┐
│  [筛选栏]  Feed: [下拉]  状态: [下拉]       │
│           标签: [输入]   排序: [下拉]        │
├─────────────────────────────────────────────┤
│  共 150 篇文档                    第 1/8 页  │
├─────────────────────────────────────────────┤
│  📄 Article Title 1                         │
│     来源: Hacker News | 标签: tech, AI       │
│     状态: ✅ 已处理 | 2026-05-29             │
│     摘要: First 200 chars...                │
├─────────────────────────────────────────────┤
│  [上一页]  1 2 3 ... 8  [下一页]            │
└─────────────────────────────────────────────┘
```

### Status Markers

- pending: "⏳ 待处理"
- processing: "🔄 处理中"
- done: "✅ 已完成"
- failed: "❌ 失败"

### Article Detail

Click article title → switch to detail view (same pattern as Wiki tab: list/content toggle). Shows full title, metadata, and complete content.

### Filtering

- Feed dropdown: populated from `/api/documents/stats` feeds list
- Status dropdown: pending / processing / done / failed
- Source dropdown: raw / cleaned_raw / reject
- Tag input: free text filter
- Sort dropdown: created_at (newest/oldest), title (A-Z/Z-A)

### Pagination

- Previous / Next buttons + page numbers
- 20 items per page

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/web/handler_documents.go` | Create | 3 API handlers |
| `internal/web/server.go` | Modify | Register 3 new routes |
| `internal/web/static/index.html` | Modify | Add Documents tab HTML |
| `internal/web/static/app.js` | Modify | Add document list/detail/filter logic |
| `internal/web/static/style.css` | Modify | Add document list styles |
| `tests/frontend.spec.ts` | Modify | Add Documents tab test cases |

## Verification

1. Build and deploy Docker container
2. Run `npx playwright test` - all tests pass
3. Manually verify at http://localhost:6006/ - Documents tab works
