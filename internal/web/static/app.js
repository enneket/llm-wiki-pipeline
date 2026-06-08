// Time formatting
function formatTime(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    // Check for zero date (0001-01-01)
    if (date.getFullYear() < 2000) return '-';
    return date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' });
}

// Tab switching
function switchTab(tabName) {
    document.querySelectorAll('.nav-item').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.page').forEach(c => c.classList.remove('active'));

    const tab = document.querySelector(`.nav-item[data-tab="${tabName}"]`);
    const content = document.getElementById(tabName);
    if (tab && content) {
        tab.classList.add('active');
        content.classList.add('active');

        // Load data when switching tabs
        if (tabName === 'status') loadStatus();
        if (tabName === 'feeds') loadFeeds();
        if (tabName === 'wiki') loadWiki();
        if (tabName === 'settings') loadSettings();
        if (tabName === 'documents') loadDocuments();
    }
}

document.querySelectorAll('.nav-item').forEach(tab => {
    tab.addEventListener('click', (e) => {
        e.preventDefault();
        const tabName = tab.dataset.tab;
        history.replaceState(null, '', '#' + tabName);
        switchTab(tabName);
    });
});

// Restore tab from URL hash on page load
window.addEventListener('hashchange', () => {
    const tabName = window.location.hash.slice(1) || 'query';
    switchTab(tabName);
});

// Initial tab load
document.addEventListener('DOMContentLoaded', () => {
    const tabName = window.location.hash.slice(1) || 'query';
    switchTab(tabName);
    
    // Check and restore progress for running tasks
    checkAndRestoreProgress();
});

// Query
async function askQuestion() {
    const question = document.getElementById('question').value.trim();
    if (!question) return;

    const btn = document.getElementById('ask-btn');
    btn.disabled = true;
    btn.textContent = '思考中...';

    try {
        const res = await fetch('/api/query', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ question })
        });

        if (!res.ok) throw new Error(await res.text());

        const data = await res.json();
        document.getElementById('answer').style.display = 'block';
        document.getElementById('answer-content').textContent = data.answer;

        const sourcesDiv = document.getElementById('sources');
        if (data.sources && data.sources.length > 0) {
            sourcesDiv.innerHTML = '<h4>参考来源</h4>' +
                data.sources.map(s => `<div class="source-item">${s.title}</div>`).join('');
        } else {
            sourcesDiv.innerHTML = '';
        }
    } catch (err) {
        alert('查询失败: ' + err.message);
    } finally {
        btn.disabled = false;
        btn.textContent = '提问';
    }
}

// Status
async function loadStatus() {
    try {
        const res = await fetch('/api/status');
        const data = await res.json();

        document.getElementById('stat-feeds').textContent = data.feeds;
        document.getElementById('stat-docs').textContent = data.documents;
        document.getElementById('stat-wiki').textContent = data.wiki_pages;
        document.getElementById('stat-pending').textContent = data.pending;
        document.getElementById('stat-processing').textContent = data.processing;
        document.getElementById('stat-done').textContent = data.done;
        document.getElementById('stat-failed').textContent = data.failed;
    } catch (err) {
        console.error('Failed to load status:', err);
    }
}

// Feeds
let allFeeds = [];
let feedState = { page: 1, perPage: 20 };

async function loadFeeds() {
    try {
        const res = await fetch('/api/feeds');
        allFeeds = await res.json();
        feedState.page = 1;
        renderFeedList();
    } catch (err) {
        console.error('Failed to load feeds:', err);
    }
}

function renderFeedList() {
    const totalItems = allFeeds.length;
    const totalPages = Math.max(1, Math.ceil(totalItems / feedState.perPage));
    if (feedState.page > totalPages) feedState.page = totalPages;

    const start = (feedState.page - 1) * feedState.perPage;
    const end = start + feedState.perPage;
    const pageFeeds = allFeeds.slice(start, end);

    const list = document.getElementById('feed-list');
    list.innerHTML = pageFeeds.map(f => `
        <div class="feed-item">
            <div class="feed-info">
                <h3>${escapeHtml(f.name)}</h3>
                <p>${escapeHtml(f.url)}</p>
                ${f.tags.length ? `<div class="feed-tags">${f.tags.map(t => `<span class="tag">${escapeHtml(t)}</span>`).join('')}</div>` : ''}
            </div>
            <div class="feed-actions">
                <button class="btn btn-ghost btn-sm" onclick="editFeed(${f.id}, '${escapeHtml(f.name)}', '${escapeHtml(f.url)}', '${f.tags.join(',')}')">编辑</button>
                <button class="btn btn-ghost btn-sm btn-danger-text" onclick="deleteFeed(${f.id})">删除</button>
            </div>
        </div>
    `).join('');

    // Pagination
    const pagDiv = document.getElementById('feed-pagination');
    let pagHtml = '';
    if (totalPages > 1) {
        if (feedState.page > 1) {
            pagHtml += `<button onclick="goFeedPage(1)">首页</button>`;
            pagHtml += `<button onclick="goFeedPage(${feedState.page - 1})">上一页</button>`;
        }
        let startPage = Math.max(1, feedState.page - 3);
        let endPage = Math.min(totalPages, feedState.page + 3);
        for (let i = startPage; i <= endPage; i++) {
            pagHtml += `<button onclick="goFeedPage(${i})" ${i === feedState.page ? 'class="active"' : ''}>${i}</button>`;
        }
        if (feedState.page < totalPages) {
            pagHtml += `<button onclick="goFeedPage(${feedState.page + 1})">下一页</button>`;
            pagHtml += `<button onclick="goFeedPage(${totalPages})">末页</button>`;
        }
    }
    pagDiv.innerHTML = pagHtml;
    pagDiv.style.display = totalPages > 1 ? 'flex' : 'none';
}

function goFeedPage(page) {
    feedState.page = page;
    renderFeedList();
}

function editFeed(id, name, url, tags) {
    document.getElementById('feed-edit-id').value = id;
    document.getElementById('feed-edit-name').value = name;
    document.getElementById('feed-edit-url').value = url;
    document.getElementById('feed-edit-tags').value = tags;
    document.getElementById('feed-edit-dialog').style.display = 'flex';
}

function cancelEditFeed() {
    document.getElementById('feed-edit-dialog').style.display = 'none';
}

async function saveEditFeed() {
    const id = document.getElementById('feed-edit-id').value;
    const name = document.getElementById('feed-edit-name').value.trim();
    const url = document.getElementById('feed-edit-url').value.trim();
    const tags = document.getElementById('feed-edit-tags').value.split(',').map(t => t.trim()).filter(Boolean);

    if (!name || !url) {
        alert('请输入名称和 URL');
        return;
    }

    try {
        const res = await fetch(`/api/feeds/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, url, tags })
        });

        if (!res.ok) throw new Error(await res.text());

        cancelEditFeed();
        loadFeeds();
    } catch (err) {
        alert('更新失败: ' + err.message);
    }
}

function showAddFeed() {
    document.getElementById('feed-add-name').value = '';
    document.getElementById('feed-add-url').value = '';
    document.getElementById('feed-add-tags').value = '';
    document.getElementById('feed-add-dialog').style.display = 'flex';
}

function cancelAddFeed() {
    document.getElementById('feed-add-dialog').style.display = 'none';
}

async function saveAddFeed() {
    const name = document.getElementById('feed-add-name').value.trim();
    const url = document.getElementById('feed-add-url').value.trim();
    const tags = document.getElementById('feed-add-tags').value.split(',').map(t => t.trim()).filter(Boolean);

    if (!name || !url) {
        alert('请输入名称和 URL');
        return;
    }

    try {
        const res = await fetch('/api/feeds', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, url, tags })
        });

        if (!res.ok) throw new Error(await res.text());

        cancelAddFeed();
        loadFeeds();
    } catch (err) {
        alert('添加失败: ' + err.message);
    }
}

async function deleteFeed(id) {
    if (!confirm('确定删除这个 Feed?')) return;

    try {
        const res = await fetch(`/api/feeds/${id}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
        loadFeeds();
    } catch (err) {
        alert('删除失败: ' + err.message);
    }
}

// Wiki
let allWikiPages = [];
let currentWikiType = 'entity';
let wikiState = { page: 1, perPage: 50 };

async function loadWiki() {
    try {
        const res = await fetch('/api/wiki');
        allWikiPages = await res.json();

        // Populate tag filter
        const tags = new Set();
        allWikiPages.forEach(p => (p.tags || []).forEach(t => tags.add(t)));
        const tagSelect = document.getElementById('wiki-filter-tag');
        const currentTag = tagSelect.value;
        tagSelect.innerHTML = '<option value="">全部分类</option>' +
            Array.from(tags).sort().map(t => `<option value="${escapeHtml(t)}">${escapeHtml(t)}</option>`).join('');
        tagSelect.value = currentTag;

        wikiState.page = 1;
        renderWikiList();
    } catch (err) {
        console.error('Failed to load wiki:', err);
    }
}

function renderWikiList() {
    const tagFilter = document.getElementById('wiki-filter-tag').value;
    const search = document.getElementById('wiki-search').value.toLowerCase();

    let filtered = allWikiPages.filter(p => p.page_type === currentWikiType);
    if (tagFilter) filtered = filtered.filter(p => (p.tags || []).includes(tagFilter));
    if (search) filtered = filtered.filter(p => p.title.toLowerCase().includes(search));

    // Group by tag
    const groups = {};
    filtered.forEach(p => {
        const mainTag = (p.tags || [])[0] || '未分类';
        if (!groups[mainTag]) groups[mainTag] = [];
        groups[mainTag].push(p);
    });

    const sortedGroups = Object.entries(groups).sort(([a], [b]) => a.localeCompare(b));
    const totalItems = filtered.length;
    const totalPages = Math.max(1, Math.ceil(totalItems / wikiState.perPage));
    if (wikiState.page > totalPages) wikiState.page = totalPages;

    const start = (wikiState.page - 1) * wikiState.perPage;
    const end = start + wikiState.perPage;

    // Paginate groups
    let count = 0;
    const pageGroups = [];
    for (const [tag, pages] of sortedGroups) {
        const groupStart = count;
        const groupEnd = count + pages.length;
        if (groupEnd > start && groupStart < end) {
            const sliceStart = Math.max(0, start - groupStart);
            const sliceEnd = Math.min(pages.length, end - groupStart);
            pageGroups.push([tag, pages.slice(sliceStart, sliceEnd)]);
        }
        count += pages.length;
        if (count >= end) break;
    }

    const list = document.getElementById('wiki-list');
    if (!filtered || filtered.length === 0) {
        list.innerHTML = '<p>暂无页面</p>';
    } else {
        list.innerHTML = pageGroups.map(([tag, pages]) => `
            <div class="wiki-group">
                <h3 class="wiki-group-title">${escapeHtml(tag)} <span class="wiki-group-count">(${(groups[tag] || []).length})</span></h3>
                <div class="wiki-group-items">
                    ${pages.map(p => `
                        <div class="wiki-item" onclick="loadWikiPage('${p.slug}')">
                            <span class="wiki-item-title">${escapeHtml(p.title)}</span>
                        </div>
                    `).join('')}
                </div>
            </div>
        `).join('');
    }

    // Pagination
    const pagDiv = document.getElementById('wiki-pagination');
    let pagHtml = '';
    if (totalPages > 1) {
        if (wikiState.page > 1) {
            pagHtml += `<button onclick="goWikiPage(1)">首页</button>`;
            pagHtml += `<button onclick="goWikiPage(${wikiState.page - 1})">上一页</button>`;
        }
        let startPage = Math.max(1, wikiState.page - 3);
        let endPage = Math.min(totalPages, wikiState.page + 3);
        if (startPage > 1) {
            pagHtml += `<button onclick="goWikiPage(1)">1</button>`;
            if (startPage > 2) pagHtml += `<span class="page-ellipsis">...</span>`;
        }
        for (let i = startPage; i <= endPage; i++) {
            pagHtml += `<button onclick="goWikiPage(${i})" ${i === wikiState.page ? 'class="active"' : ''}>${i}</button>`;
        }
        if (endPage < totalPages) {
            if (endPage < totalPages - 1) pagHtml += `<span class="page-ellipsis">...</span>`;
            pagHtml += `<button onclick="goWikiPage(${totalPages})">${totalPages}</button>`;
        }
        if (wikiState.page < totalPages) {
            pagHtml += `<button onclick="goWikiPage(${wikiState.page + 1})">下一页</button>`;
            pagHtml += `<button onclick="goWikiPage(${totalPages})">末页</button>`;
        }
    }
    pagDiv.innerHTML = pagHtml;
    pagDiv.style.display = totalPages > 1 ? 'flex' : 'none';

    document.getElementById('wiki-list').style.display = 'block';
    document.getElementById('wiki-content').style.display = 'none';
}

function goWikiPage(page) {
    wikiState.page = page;
    renderWikiList();
}

async function loadWikiPage(slug) {
    try {
        const res = await fetch(`/api/wiki/${slug}`);
        if (!res.ok) throw new Error('Page not found');

        const page = await res.json();
        document.getElementById('wiki-title').textContent = page.title;
        document.getElementById('wiki-body').innerHTML = renderMarkdown(page.content);
        document.getElementById('wiki-list').style.display = 'none';
        document.getElementById('wiki-content').style.display = 'block';
    } catch (err) {
        alert('加载失败: ' + err.message);
    }
}

function backToList() {
    document.getElementById('wiki-list').style.display = 'block';
    document.getElementById('wiki-content').style.display = 'none';
}

// Simple markdown rendering
function renderMarkdown(text) {
    // Strip YAML frontmatter
    let content = text;
    if (content.startsWith('---')) {
        const endIdx = content.indexOf('---', 3);
        if (endIdx !== -1) {
            content = content.substring(endIdx + 3).trim();
        }
    }

    return content
        .replace(/^### (.*$)/gm, '<h3>$1</h3>')
        .replace(/^## (.*$)/gm, '<h2>$1</h2>')
        .replace(/^# (.*$)/gm, '<h1>$1</h1>')
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code>$1</code>')
        .replace(/\n/g, '<br>');
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Feed Import/Export
document.getElementById('import-file').addEventListener('change', async (e) => {
    const file = e.target.files[0];
    if (!file) return;

    const formData = new FormData();
    formData.append('file', file);

    try {
        const res = await fetch('/api/feeds/import', {
            method: 'POST',
            body: formData
        });

        if (!res.ok) throw new Error(await res.text());

        const data = await res.json();
        alert(`导入成功: 新增 ${data.added} 个 Feed`);
        e.target.value = '';
        loadFeeds();
    } catch (err) {
        alert('导入失败: ' + err.message);
    }
});

function exportFeeds(format) {
    window.location.href = `/api/feeds/export?format=${format}`;
}

let fetchPollInterval = null;

async function fetchFeeds() {
    try {
        const res = await fetch('/api/feeds/fetch', { method: 'POST' });
        if (!res.ok) {
            const err = await res.text();
            if (err.includes('already in progress')) {
                alert('拉取正在进行中');
            } else {
                throw new Error(err);
            }
            return;
        }
        startFetchProgress();
    } catch (err) {
        alert('拉取失败: ' + err.message);
    }
}

function startFetchProgress() {
    const progressDiv = document.getElementById('fetch-progress');
    const progressText = document.getElementById('progress-text');
    const fetchBtn = document.getElementById('fetch-btn');

    progressDiv.style.display = 'flex';
    progressText.textContent = '准备中...';
    fetchBtn.disabled = true;
    fetchBtn.textContent = '拉取中...';

    fetchPollInterval = setInterval(async () => {
        try {
            const res = await fetch('/api/feeds/fetch/status');
            const data = await res.json();

            if (!data.running) {
                stopFetchProgress();
                loadFeeds();
                return;
            }

            const percent = data.total > 0 ? Math.round((data.completed / data.total) * 100) : 0;
            progressText.textContent = data.current 
                ? `${percent}% ${data.completed}/${data.total} - ${data.current}` 
                : `${percent}% ${data.completed}/${data.total}`;
        } catch (err) {
            console.error('Failed to fetch progress:', err);
        }
    }, 300);
}

function stopFetchProgress() {
    if (fetchPollInterval) {
        clearInterval(fetchPollInterval);
        fetchPollInterval = null;
    }

    const progressDiv = document.getElementById('fetch-progress');
    const fetchBtn = document.getElementById('fetch-btn');

    progressDiv.style.display = 'none';
    fetchBtn.disabled = false;
    fetchBtn.textContent = '立即拉取';
}

// Settings
async function loadSettings() {
    try {
        const res = await fetch('/api/settings');
        const settings = await res.json();
        console.log('Loaded settings:', settings); // Debug log

        // LLM
        if (settings.llm) {
            console.log('LLM settings:', settings.llm); // Debug log
            document.getElementById('llm-model').value = settings.llm.model || '';
            document.getElementById('llm-base-url').value = settings.llm.base_url || '';
            document.getElementById('llm-api-key').value = settings.llm.api_key || '';
        }

        // Filter
        if (settings.filter) {
            document.getElementById('filter-mode').value = settings.filter.mode || 'keyword';
            if (settings.filter.keyword) {
                renderFilterTags(settings.filter.keyword.tags || []);
                renderBlacklistTags(settings.filter.keyword.blacklist_tags || []);
                document.getElementById('filter-keyword-match-any').checked = settings.filter.keyword.match_any || false;
            }
        }

        // Dedup
        if (settings.dedup) {
            document.getElementById('dedup-url-exact').checked = settings.dedup.url_exact || false;
            document.getElementById('dedup-content-hash').checked = settings.dedup.content_hash || false;
            document.getElementById('dedup-embedding-context').checked = settings.dedup.embedding_context || false;
            if (settings.dedup.vector) {
                console.log('Vector settings:', settings.dedup.vector); // Debug log
                document.getElementById('dedup-vector-enabled').checked = settings.dedup.vector.enabled || false;
                document.getElementById('dedup-vector-threshold').value = settings.dedup.vector.threshold || 0.85;
                document.getElementById('dedup-vector-model').value = settings.dedup.vector.model || '';
                document.getElementById('dedup-vector-url').value = settings.dedup.vector.embedding_url || '';
                document.getElementById('dedup-vector-key').value = settings.dedup.vector.embedding_api_key || '';
            }
        }

        // General
        if (settings.general) {
            document.getElementById('general-interval').value = settings.general.interval || '';
            document.getElementById('general-concurrency').value = settings.general.concurrency || 1;
        }
    } catch (err) {
        console.error('Failed to load settings:', err);
    }
}

async function saveSettings(category) {
    let data = {};

    switch (category) {
        case 'llm':
            data = {
                model: document.getElementById('llm-model').value,
                api_key: document.getElementById('llm-api-key').value,
                base_url: document.getElementById('llm-base-url').value
            };
            // Don't send empty api_key (preserve existing)
            if (!data.api_key) delete data.api_key;
            break;
        case 'filter':
            data = {
                mode: document.getElementById('filter-mode').value,
                keyword: {
                    match_any: document.getElementById('filter-keyword-match-any').checked,
                    tags: getFilterTags(),
                    blacklist_tags: getBlacklistTags()
                }
            };
            break;
        case 'dedup':
            data = {
                url_exact: document.getElementById('dedup-url-exact').checked,
                content_hash: document.getElementById('dedup-content-hash').checked,
                embedding_context: document.getElementById('dedup-embedding-context').checked,
                vector: {
                    enabled: document.getElementById('dedup-vector-enabled').checked,
                    threshold: parseFloat(document.getElementById('dedup-vector-threshold').value) || 0.85,
                    model: document.getElementById('dedup-vector-model').value,
                    embedding_url: document.getElementById('dedup-vector-url').value,
                    embedding_api_key: document.getElementById('dedup-vector-key').value
                }
            };
            // Don't send empty api_key (preserve existing)
            // Send empty strings to clear values
            break;
        case 'general':
            data = {
                interval: document.getElementById('general-interval').value,
                concurrency: parseInt(document.getElementById('general-concurrency').value) || 1
            };
            break;
    }

    try {
        const res = await fetch(`/api/settings/${category}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        if (!res.ok) {
            const err = await res.text();
            throw new Error(err);
        }

        alert('配置已保存');
    } catch (err) {
        alert('保存失败: ' + err.message);
    }
}

function toggleApiKey(inputId) {
    const input = document.getElementById(inputId);
    const btn = input.nextElementSibling;
    if (input.type === 'password') {
        input.type = 'text';
        btn.textContent = '隐藏';
    } else {
        input.type = 'password';
        btn.textContent = '显示';
    }
}

async function testLLM() {
    const resultDiv = document.getElementById('llm-test-result');
    resultDiv.style.display = 'block';
    resultDiv.className = 'test-result';
    resultDiv.textContent = '测试中...';

    try {
        const res = await fetch('/api/settings/test-llm', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Test failed');
        resultDiv.className = 'test-result success';
        resultDiv.textContent = `测试成功！响应: ${data.response}`;
    } catch (err) {
        resultDiv.className = 'test-result error';
        resultDiv.textContent = `测试失败: ${err.message}`;
    }
}

async function testEmbedding() {
    const resultDiv = document.getElementById('embedding-test-result');
    resultDiv.style.display = 'block';
    resultDiv.className = 'test-result';
    resultDiv.textContent = '测试中...';

    try {
        const res = await fetch('/api/settings/test-embedding', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Test failed');
        resultDiv.className = 'test-result success';
        resultDiv.textContent = `测试成功！向量维度: ${data.dimension}`;
    } catch (err) {
        resultDiv.className = 'test-result error';
        resultDiv.textContent = `测试失败: ${err.message}`;
    }
}

// Keyboard shortcut
document.getElementById('question').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        askQuestion();
    }
});

// Embeddings
let embedPollInterval = null;

async function rebuildEmbeddings() {
    try {
        const res = await fetch('/api/embeddings/rebuild', { method: 'POST' });
        if (!res.ok) {
            const err = await res.text();
            throw new Error(err);
        }
        startEmbedProgress();
    } catch (err) {
        alert('重建失败: ' + err.message);
    }
}

async function stopEmbeddings() {
    try {
        await fetch('/api/embeddings/stop', { method: 'POST' });
    } catch (err) {
        alert('停止失败: ' + err.message);
    }
}

function startEmbedProgress() {
    const progressDiv = document.getElementById('embed-progress');
    const progressText = document.getElementById('embed-progress-text');
    const embedBtn = document.getElementById('embed-btn');
    const stopBtn = document.getElementById('embed-stop-btn');

    progressDiv.style.display = 'flex';
    progressText.textContent = '准备中...';
    embedBtn.disabled = true;
    embedBtn.textContent = '重建中...';
    stopBtn.style.display = 'inline-block';

    embedPollInterval = setInterval(async () => {
        try {
            const res = await fetch('/api/embeddings/status');
            const data = await res.json();

            if (!data.running) {
                stopEmbedProgress();
                return;
            }

            const percent = data.total > 0 ? Math.round((data.completed / data.total) * 100) : 0;
            progressText.textContent = data.current 
                ? `${percent}% ${data.completed}/${data.total} - ${data.current}` 
                : `${percent}% ${data.completed}/${data.total}`;
        } catch (err) {
            console.error('Failed to fetch embed status:', err);
        }
    }, 500);
}

function stopEmbedProgress() {
    if (embedPollInterval) {
        clearInterval(embedPollInterval);
        embedPollInterval = null;
    }

    const progressDiv = document.getElementById('embed-progress');
    const embedBtn = document.getElementById('embed-btn');
    const stopBtn = document.getElementById('embed-stop-btn');

    progressDiv.style.display = 'none';
    embedBtn.disabled = false;
    embedBtn.textContent = '重建 Embeddings';
    stopBtn.style.display = 'none';
}

// Check embed progress on load
async function checkAndRestoreProgress() {
    try {
        const [fetchRes, processRes, embedRes] = await Promise.all([
            fetch('/api/feeds/fetch/status'),
            fetch('/api/documents/process/status'),
            fetch('/api/embeddings/status')
        ]);

        if (!fetchRes.ok) throw new Error(`HTTP ${fetchRes.status}`);
        if (!processRes.ok) throw new Error(`HTTP ${processRes.status}`);
        if (!embedRes.ok) throw new Error(`HTTP ${embedRes.status}`);

        const [fetchData, processData, embedData] = await Promise.all([
            fetchRes.json(),
            processRes.json(),
            embedRes.json()
        ]);

        if (fetchData.running) {
            startFetchProgress();
        }

        if (processData.running) {
            startProcessProgress();
        }

        if (embedData.running) {
            startEmbedProgress();
        }
    } catch (err) {
        console.error('Failed to check task status:', err);
    }
}
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
        const statusMap = { pending: '⏳ 待处理', processing: '🔄 处理中', done: '✅ 已完成', failed: '❌ 失败' };

        // Group by date
        const groups = {};
        items.forEach(d => {
            const publishedTime = d.published && new Date(d.published).getFullYear() >= 2000 ? d.published : null;
            const displayTime = publishedTime || d.created_at;
            const dateStr = displayTime ? new Date(displayTime).toLocaleDateString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '未知日期';
            if (!groups[dateStr]) groups[dateStr] = [];
            groups[dateStr].push(d);
        });

        list.innerHTML = Object.entries(groups).map(([date, docs]) => `
            <div class="doc-group">
                <h3 class="doc-group-title">${escapeHtml(date)}</h3>
                <div class="doc-group-items">
                    ${docs.map(d => {
                        const publishedTime = d.published && new Date(d.published).getFullYear() >= 2000 ? d.published : null;
                        const displayTime = publishedTime || d.created_at;
                        return `
                        <div class="doc-item">
                            <div class="doc-item-header">
                                <h3><a href="#" onclick="loadDocPage(${d.id}); return false;">${escapeHtml(d.title || '无标题')}</a></h3>
                                ${d.status === 'failed' ? `<button class="retry-btn" onclick="retryDocument(${d.id})">重试</button>` : ''}
                            </div>
                            <div class="doc-item-meta">
                                <span>来源: ${escapeHtml(d.feed_name || '-')}</span>
                                <span>${statusMap[d.status] || d.status}</span>
                                <span>${formatTime(displayTime)}</span>
                            </div>
                        </div>`;
                    }).join('')}
                </div>
            </div>
        `).join('');

        // Pagination
        const pagDiv = document.getElementById('doc-pagination');
        let pagHtml = '';
        if (data.page > 1) {
            pagHtml += `<button onclick="goDocPage(1)">首页</button>`;
            pagHtml += `<button onclick="goDocPage(${data.page - 1})">上一页</button>`;
        }
        
        // Show pages around current page
        let startPage = Math.max(1, data.page - 5);
        let endPage = Math.min(totalPages, data.page + 5);
        
        if (startPage > 1) {
            pagHtml += `<button onclick="goDocPage(1)">1</button>`;
            if (startPage > 2) pagHtml += `<span class="page-ellipsis">...</span>`;
        }
        
        for (let i = startPage; i <= endPage; i++) {
            pagHtml += `<button onclick="goDocPage(${i})" ${i === data.page ? 'class="active"' : ''}>${i}</button>`;
        }
        
        if (endPage < totalPages) {
            if (endPage < totalPages - 1) pagHtml += `<span class="page-ellipsis">...</span>`;
            pagHtml += `<button onclick="goDocPage(${totalPages})">${totalPages}</button>`;
        }
        
        if (data.page < totalPages) {
            pagHtml += `<button onclick="goDocPage(${data.page + 1})">下一页</button>`;
            pagHtml += `<button onclick="goDocPage(${totalPages})">末页</button>`;
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

async function retryDocument(id) {
    try {
        const res = await fetch(`/api/documents/${id}/retry`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        fetchDocuments();
    } catch (err) {
        alert('重试失败: ' + err.message);
    }
}

async function loadDocPage(id) {
    try {
        const res = await fetch(`/api/documents/${id}`);
        if (!res.ok) throw new Error('Not found');
        const doc = await res.json();

        document.getElementById('doc-title').textContent = doc.title;
        document.getElementById('doc-meta-feed').textContent = '来源: ' + (doc.feed_name || '-');
        const statusMap = { pending: '⏳ 待处理', processing: '🔄 处理中', done: '✅ 已完成', failed: '❌ 失败' };
        document.getElementById('doc-meta-status').textContent = statusMap[doc.status] || doc.status;
        document.getElementById('doc-meta-source').textContent = '类型: ' + doc.source;
        document.getElementById('doc-meta-date').textContent = formatTime(doc.created_at);
        document.getElementById('doc-body').innerHTML = renderMarkdown(doc.content);

        // Original link
        const linkEl = document.getElementById('doc-original-link');
        if (doc.url) {
            linkEl.href = doc.url;
            linkEl.style.display = 'inline-flex';
        } else {
            linkEl.style.display = 'none';
        }

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

// Document processing
let processPollInterval = null;

async function processDocuments() {
    try {
        const res = await fetch('/api/documents/process', { method: 'POST' });
        if (!res.ok) {
            const err = await res.text();
            if (err.includes('already in progress')) {
                alert('处理正在进行中');
            } else {
                throw new Error(err);
            }
            return;
        }
        startProcessProgress();
    } catch (err) {
        alert('处理失败: ' + err.message);
    }
}

async function stopProcessDocuments() {
    try {
        const res = await fetch('/api/documents/process/stop', { method: 'POST' });
        if (!res.ok) {
            const err = await res.text();
            throw new Error(err);
        }
    } catch (err) {
        alert('停止失败: ' + err.message);
    }
}

function startProcessProgress() {
    const progressDiv = document.getElementById('process-progress');
    const progressText = document.getElementById('process-progress-text');
    const processBtn = document.getElementById('process-btn');
    const stopBtn = document.getElementById('process-stop-btn');

    progressDiv.style.display = 'flex';
    progressText.textContent = '准备中...';
    processBtn.disabled = true;
    processBtn.textContent = '处理中...';
    stopBtn.style.display = 'inline-block';

    processPollInterval = setInterval(async () => {
        try {
            const res = await fetch('/api/documents/process/status');
            const data = await res.json();

            if (!data.running) {
                stopProcessProgress();
                fetchDocuments();
                return;
            }

            const percent = data.total > 0 ? Math.round((data.completed / data.total) * 100) : 0;
            progressText.textContent = data.current 
                ? `${percent}% ${data.completed}/${data.total} - ${data.current}` 
                : `${percent}% ${data.completed}/${data.total}`;
        } catch (err) {
            console.error('Failed to fetch process status:', err);
        }
    }, 300);
}

function stopProcessProgress() {
    if (processPollInterval) {
        clearInterval(processPollInterval);
        processPollInterval = null;
    }

    const progressDiv = document.getElementById('process-progress');
    const processBtn = document.getElementById('process-btn');
    const stopBtn = document.getElementById('process-stop-btn');

    progressDiv.style.display = 'none';
    processBtn.disabled = false;
    processBtn.textContent = 'LLM 处理';
    stopBtn.style.display = 'none';
}

// Filter Tags Management
function renderFilterTags(tags) {
    const list = document.getElementById('filter-tags-list');
    list.innerHTML = '';
    tags.forEach(tag => addFilterTagChip(tag, 'filter-tags-list'));
}

function renderBlacklistTags(tags) {
    const list = document.getElementById('filter-blacklist-tags-list');
    list.innerHTML = '';
    tags.forEach(tag => addFilterTagChip(tag, 'filter-blacklist-tags-list'));
}

function addFilterTagChip(tag, listId) {
    const list = document.getElementById(listId || 'filter-tags-list');
    const chip = document.createElement('span');
    chip.className = 'tag-chip';
    chip.innerHTML = `<span>${escapeHtml(tag)}</span><span class="tag-remove" onclick="removeFilterTag(this)">×</span>`;
    list.appendChild(chip);
}

function removeFilterTag(el) {
    el.parentElement.remove();
}

function getFilterTags() {
    const chips = document.querySelectorAll('#filter-tags-list .tag-chip');
    return Array.from(chips).map(c => c.querySelector('span').textContent.trim());
}

function getBlacklistTags() {
    const chips = document.querySelectorAll('#filter-blacklist-tags-list .tag-chip');
    return Array.from(chips).map(c => c.querySelector('span').textContent.trim());
}

// Init tags input
document.addEventListener('DOMContentLoaded', () => {
    // Whitelist tags input
    const input = document.getElementById('filter-keyword-tags-input');
    const container = document.getElementById('filter-tags-container');

    if (input) {
        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ',') {
                e.preventDefault();
                const val = input.value.trim().replace(/,$/g, '');
                if (val) {
                    addFilterTagChip(val, 'filter-tags-list');
                    input.value = '';
                }
            }
            if (e.key === 'Backspace' && input.value === '') {
                const chips = document.querySelectorAll('#filter-tags-list .tag-chip');
                if (chips.length > 0) chips[chips.length - 1].remove();
            }
        });
    }

    if (container) {
        container.addEventListener('click', () => input && input.focus());
    }

    // Blacklist tags input
    const blInput = document.getElementById('filter-blacklist-tags-input');
    const blContainer = document.getElementById('filter-blacklist-container');

    if (blInput) {
        blInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ',') {
                e.preventDefault();
                const val = blInput.value.trim().replace(/,$/g, '');
                if (val) {
                    addFilterTagChip(val, 'filter-blacklist-tags-list');
                    blInput.value = '';
                }
            }
            if (e.key === 'Backspace' && blInput.value === '') {
                const chips = document.querySelectorAll('#filter-blacklist-tags-list .tag-chip');
                if (chips.length > 0) chips[chips.length - 1].remove();
            }
        });
    }

    if (blContainer) {
        blContainer.addEventListener('click', () => blInput && blInput.focus());
    }
});

// Wiki tabs and filters
document.querySelectorAll('.wiki-tab').forEach(tab => {
    tab.addEventListener('click', () => {
        document.querySelectorAll('.wiki-tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        currentWikiType = tab.dataset.wiki;
        renderWikiList();
    });
});
['wiki-filter-tag'].forEach(id => {
    document.getElementById(id).addEventListener('change', renderWikiList);
});
document.getElementById('wiki-search').addEventListener('input', renderWikiList);

// Settings Tabs
document.querySelectorAll('.settings-tab').forEach(tab => {
    tab.addEventListener('click', () => {
        document.querySelectorAll('.settings-tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.settings-panel').forEach(p => p.classList.remove('active'));
        tab.classList.add('active');
        document.getElementById('settings-' + tab.dataset.settings).classList.add('active');
    });
});

// Suggested Tags
async function loadSuggestedTags() {
    try {
        const [tagsRes, statsRes] = await Promise.all([
            fetch('/api/suggested-tags'),
            fetch('/api/suggested-tags/stats')
        ]);

        const tags = await tagsRes.json();
        const stats = await statsRes.json();

        document.getElementById('suggested-tags-count').textContent = stats.pending || 0;

        const list = document.getElementById('suggested-tags-list');
        if (!tags || tags.length === 0) {
            list.innerHTML = '<p class="text-muted">暂无推荐标签</p>';
            return;
        }

        list.innerHTML = tags.map(t => `
            <div class="suggested-tag" data-id="${t.id}">
                <span>${escapeHtml(t.tag)}</span>
                <span class="text-muted">(${t.source_count})</span>
                <div class="suggested-tag-actions">
                    <button class="suggested-tag-btn accept" onclick="acceptSuggestedTag(${t.id})" title="加入筛选">✓</button>
                    <button class="suggested-tag-btn reject" onclick="rejectSuggestedTag(${t.id})" title="忽略">✕</button>
                </div>
            </div>
        `).join('');
    } catch (err) {
        console.error('Failed to load suggested tags:', err);
    }
}

async function acceptSuggestedTag(id) {
    try {
        const res = await fetch(`/api/suggested-tags/${id}/accept`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();

        // Add to filter tags UI
        addFilterTagChip(data.tag, 'filter-tags-list');

        // Reload suggested tags
        loadSuggestedTags();
    } catch (err) {
        alert('操作失败: ' + err.message);
    }
}

async function rejectSuggestedTag(id) {
    try {
        const res = await fetch(`/api/suggested-tags/${id}/reject`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());

        // Remove from UI
        const el = document.querySelector(`.suggested-tag[data-id="${id}"]`);
        if (el) el.remove();

        // Update count
        const countEl = document.getElementById('suggested-tags-count');
        countEl.textContent = Math.max(0, parseInt(countEl.textContent) - 1);
    } catch (err) {
        alert('操作失败: ' + err.message);
    }
}

// Load suggested tags when filter settings tab is shown
document.querySelector('.settings-tab[data-settings="filter"]').addEventListener('click', () => {
    loadSuggestedTags();
});
