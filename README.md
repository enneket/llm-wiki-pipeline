# llm-wiki-pipeline

RSS 订阅 → LLM 知识库自动化 pipeline，将网页内容转化为结构化 wiki 页面。

## 架构

```
RSS Feed → Fetcher → Filter → Dedup → LLM Analyze → LLM Generate → Wiki
```

| 阶段 | 组件 | 说明 |
|------|------|------|
| step1 | Fetcher + Scheduler | RSS/Atom 抓取，cron 调度 |
| step2 | Filter + Dedup | 关键词/LLM 筛选，URL/Hash/Vector 去重 |
| step3 | Ingest + WikiWriter | LLM 分析+生成，写入 wiki 页面 |
| pgvector | 向量相似度 | wiki 页面相似度检索 |
| PostgreSQL | 状态追踪 | feed/item 处理状态记录 |

## Quick Start

```bash
# 1. 配置环境变量
cp .env.example .env
# 编辑 .env 填入 LLM_API_KEY 等

# 2. 配置 feed 源
cp config/feeds.example.yaml config/feeds.yaml
# 编辑 config/feeds.yaml

# 3. 启动 pipeline
make run        # 或: go run ./cmd/llm-wiki start
```

## 配置

所有配置在 `config/` 目录下，支持 `${ENV_VAR:-default}` 环境变量展开。

```yaml
# config/feeds.yaml
feeds:
  - name: hn-ai
    url: https://hnrss.org/frontpage
    tags: [AI, LLM]
interval: '*/10 * * * *'   # cron 表达式

# config/filter.yaml
filter:
  mode: keyword             # keyword | llm_judgment
  keyword:
    match_any: true
    tags: ${FILTER_TAGS:-AI,LLM,Rust}

# config/dedup.yaml
dedup:
  url_exact: true
  content_hash: true
  vector:
    enabled: true
    threshold: 0.85
    model: ${EMBEDDING_MODEL:-text-embedding-3-small}

# config/llm.yaml
llm:
  provider: openai          # openai | volcengine
  model: ${LLM_MODEL:-gpt-4o}
  api_key: ${LLM_API_KEY}
  base_url: ${LLM_BASE_URL:-https://api.openai.com/v1}
```

### 环境变量

```bash
LLM_API_KEY=           # LLM API Key
LLM_MODEL=             # 模型名，如 gpt-4o 或 doubao-seed-1-8-251228
LLM_BASE_URL=          # API 端点，volcengine 用 https://ark.cn-beijing.volces.com/api/v3/responses
EMBEDDING_MODEL=      # 向量模型，默认 text-embedding-3-small
EMBEDDING_BASE_URL=    # 向量 API 端点（可选，默认用 LLM_BASE_URL）
EMBEDDING_API_KEY=     # 向量 API Key（可选，默认用 LLM_API_KEY）
FILTER_TAGS=           # 逗号分隔的筛选标签，如 AI,LLM,Rust
DATABASE_URL=          # PostgreSQL 连接串，默认 postgres://.../llm_wiki?sslmode=disable
```

## CLI 命令

```bash
# Feed 管理
llm-wiki feed list                  # 查看所有 feed
llm-wiki feed add <name> <url>     # 添加单个 feed
llm-wiki feed import <file.opml>    # 批量导入 OPML 或 URL 列表
llm-wiki feed fetch                 # 手动触发一次抓取
llm-wiki feed status                # 查看状态

# Wiki 管理
llm-wiki wiki lint                  # 检查 wiki 一致性
llm-wiki wiki index                 # 重建索引
llm-wiki wiki ingest                # 手动触发 ingest
llm-wiki wiki status                # 查看队列状态

# Pipeline
llm-wiki start                      # 后台启动 scheduler
llm-wiki query "<问题>"             # 语义搜索 wiki

# 其他
llm-wiki reload                     # 热重载配置
```

## 输出

处理后的 wiki 页面保存在 `data/wiki/pages/`：

- **source 页面**：原始文章摘要
- **entity 页面**：实体（人物/组织/技术）
- **concept 页面**：概念和主题

frontmatter 示例：

```yaml
---
title: GPT-4
type: entity
tags: [AI, LLM, OpenAI]
summary: GPT-4 是 OpenAI 的多模态大语言模型
---
```

## 数据目录

```
data/
├── feeds/          # 原始 RSS 抓取
├── cleaned_raw/   # 过滤去重后的文档
├── wiki/
│   ├── pages/     # 生成的 wiki 页面
│   └── index.md   # 页面索引
```

## 开发

```bash
make build    # 编译
make run      # 运行
make test     # 测试
make lint     # 检查
```