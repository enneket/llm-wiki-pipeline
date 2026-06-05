# 标签导航页设计

## 目标

改善 wiki 导航体验。当前 405 entities + 726 concepts 全部平铺在各自目录下，难以浏览。通过基于 frontmatter tags 自动生成分类导航页，让用户可以按领域/主题快速找到相关内容。

## 设计

### 1. 改进 LLM 分析 — 更好的 tags

**当前问题：** `cot.go` 的 analyze 步骤返回 entities/concepts，但生成页面时 tags 只是名字本身（如 entity "Anthropic" 的 tags 就是 `["Anthropic"]`），没有分类意义。

**改动：** 在 `analysisResult` 中新增 `Categories []string` 字段，让 LLM 在分析时返回领域级分类标签（如 `["AI", "security", "programming"]`）。

修改 `internal/step3/cot.go` 的 analyze prompt，要求 LLM 额外返回 `categories` 字段。

### 2. 自动生成分类导航页

新增 `internal/wiki/categories.go`：

- 遍历 `data/wiki/` 下所有 `.md` 文件
- 解析 YAML frontmatter 中的 `tags` 字段
- 按 tag 聚合，生成 `data/wiki/categories/{tag_slug}.md`
- 每个分类页列出该 tag 下的所有 entities、concepts、sources

输出示例 `data/wiki/categories/ai.md`：
```markdown
---
title: "AI"
type: category
---
# AI

## Entities
- [[anthropic]]
- [[openai]]

## Concepts
- [[ai_coding_assistants]]
- [[llm_performance_ranking]]

## Sources
- [[anthropic_raises_65b_...]]
```

### 3. 集成到 ingest 流程

在 `internal/step3/ingest.go` 的 Process 方法中，每次写入 wiki 页面后，自动调用分类页更新。

### 不做的事

- 不加 CLI 命令
- 不改物理目录结构
- 不迁移现有文件

## 涉及文件

| 文件 | 改动 |
|------|------|
| `internal/step3/cot.go` | analyze prompt 增加 categories 输出 |
| `internal/step3/ingest.go` | Process 中使用 categories 作为 tags，写入后触发分类更新 |
| `internal/wiki/categories.go` | 新增：扫描 frontmatter 生成分类导航页 |
