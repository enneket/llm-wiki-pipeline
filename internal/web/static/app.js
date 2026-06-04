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
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

    const tab = document.querySelector(`.tab[data-tab="${tabName}"]`);
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

document.querySelectorAll('.tab').forEach(tab => {
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
        document.getElementById('stat-failed').textContent = data.failed;
    } catch (err) {
        console.error('Failed to load status:', err);
    }
}

// Feeds
async function loadFeeds() {
    try {
        const res = await fetch('/api/feeds');
        const feeds = await res.json();

        const list = document.getElementById('feed-list');
        list.innerHTML = feeds.map(f => `
            <div class="feed-item">
                <div class="feed-info">
                    <h3>${escapeHtml(f.name)}</h3>
                    <p>${escapeHtml(f.url)}</p>
                    ${f.tags.length ? `<p>标签: ${f.tags.join(', ')}</p>` : ''}
                </div>
                <div class="feed-actions">
                    <button class="edit-btn" onclick="editFeed(${f.id}, '${escapeHtml(f.name)}', '${escapeHtml(f.url)}', '${f.tags.join(',')}')">编辑</button>
                    <button class="delete-btn" onclick="deleteFeed(${f.id})">删除</button>
                </div>
            </div>
        `).join('');
    } catch (err) {
        console.error('Failed to load feeds:', err);
    }
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
async function loadWiki() {
    try {
        const res = await fetch('/api/wiki');
        const pages = await res.json();

        const list = document.getElementById('wiki-list');
        if (!pages || pages.length === 0) {
            list.innerHTML = '<p>暂无 Wiki 页面</p>';
        } else {
            list.innerHTML = pages.map(p => `
                <div class="wiki-item" onclick="loadWikiPage('${p.slug}')">
                    <h3>${escapeHtml(p.title)}</h3>
                    <div class="tags">
                        ${(p.tags || []).map(t => `<span class="tag">${escapeHtml(t)}</span>`).join('')}
                    </div>
                </div>
            `).join('');
        }

        document.getElementById('wiki-list').style.display = 'block';
        document.getElementById('wiki-content').style.display = 'none';
    } catch (err) {
        console.error('Failed to load wiki:', err);
    }
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
    return text
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
    const progressFill = document.getElementById('progress-fill');
    const progressText = document.getElementById('progress-text');
    const fetchBtn = document.getElementById('fetch-btn');

    progressDiv.style.display = 'flex';
    progressFill.style.width = '0%';
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
            progressFill.style.width = percent + '%';
            progressText.textContent = data.current 
                ? `${data.completed}/${data.total} - ${data.current}` 
                : `${data.completed}/${data.total}`;
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

        // LLM
        if (settings.llm) {
            document.getElementById('llm-model').value = settings.llm.model || '';
            document.getElementById('llm-base-url').value = settings.llm.base_url || '';
            document.getElementById('llm-api-key').value = settings.llm.api_key || '';
        }

        // Filter
        if (settings.filter) {
            document.getElementById('filter-mode').value = settings.filter.mode || 'keyword';
            if (settings.filter.keyword) {
                document.getElementById('filter-keyword-tags').value = (settings.filter.keyword.tags || []).join(',');
                document.getElementById('filter-keyword-match-any').checked = settings.filter.keyword.match_any || false;
            }
        }

        // Dedup
        if (settings.dedup) {
            document.getElementById('dedup-url-exact').checked = settings.dedup.url_exact || false;
            document.getElementById('dedup-content-hash').checked = settings.dedup.content_hash || false;
            if (settings.dedup.vector) {
                document.getElementById('dedup-vector-enabled').checked = settings.dedup.vector.enabled || false;
                document.getElementById('dedup-vector-threshold').value = settings.dedup.vector.threshold || 0.85;
                document.getElementById('dedup-vector-model').value = settings.dedup.vector.model || '';
                document.getElementById('dedup-vector-url').value = settings.dedup.vector.embedding_url || '';
                // Don't set api_key value for security
            }
        }

        // General
        if (settings.general) {
            document.getElementById('general-interval').value = settings.general.interval || '';
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
                    tags: document.getElementById('filter-keyword-tags').value.split(',').map(t => t.trim()).filter(Boolean)
                }
            };
            break;
        case 'dedup':
            data = {
                url_exact: document.getElementById('dedup-url-exact').checked,
                content_hash: document.getElementById('dedup-content-hash').checked,
                vector: {
                    enabled: document.getElementById('dedup-vector-enabled').checked,
                    threshold: parseFloat(document.getElementById('dedup-vector-threshold').value) || 0.85,
                    model: document.getElementById('dedup-vector-model').value,
                    embedding_url: document.getElementById('dedup-vector-url').value,
                    embedding_api_key: document.getElementById('dedup-vector-key').value
                }
            };
            // Don't send empty api_key (preserve existing)
            if (!data.vector.embedding_api_key) delete data.vector.embedding_api_key;
            break;
        case 'general':
            data = {
                interval: document.getElementById('general-interval').value
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
            const publishedTime = d.published && new Date(d.published).getFullYear() >= 2000 ? d.published : null;
            const displayTime = publishedTime || d.created_at;
            return `
                <div class="doc-item">
                    <h3><a href="#" onclick="loadDocPage(${d.id}); return false;">${escapeHtml(d.title || '无标题')}</a></h3>
                    <div class="doc-item-meta">
                        <span>来源: ${escapeHtml(d.feed_name || '-')}</span>
                        <span>${statusMap[d.status] || d.status}</span>
                        <span>${formatTime(displayTime)}</span>
                    </div>
                </div>
            `;
        }).join('');

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

function startProcessProgress() {
    const progressDiv = document.getElementById('process-progress');
    const progressFill = document.getElementById('process-progress-fill');
    const progressText = document.getElementById('process-progress-text');
    const processBtn = document.getElementById('process-btn');

    progressDiv.style.display = 'flex';
    progressFill.style.width = '0%';
    progressText.textContent = '准备中...';
    processBtn.disabled = true;
    processBtn.textContent = '处理中...';

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
            progressFill.style.width = percent + '%';
            progressText.textContent = data.current 
                ? `${data.completed}/${data.total} - ${data.current}` 
                : `${data.completed}/${data.total}`;
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

    progressDiv.style.display = 'none';
    processBtn.disabled = false;
    processBtn.textContent = 'LLM 处理';
}

async function checkAndRestoreProgress() {
    try {
        const [fetchRes, processRes] = await Promise.all([
            fetch('/api/feeds/fetch/status'),
            fetch('/api/documents/process/status')
        ]);

        if (!fetchRes.ok) throw new Error(`HTTP ${fetchRes.status}`);
        if (!processRes.ok) throw new Error(`HTTP ${processRes.status}`);

        const [fetchData, processData] = await Promise.all([
            fetchRes.json(),
            processRes.json()
        ]);

        if (fetchData.running) {
            startFetchProgress();
        }

        if (processData.running) {
            startProcessProgress();
        }
    } catch (err) {
        console.error('Failed to check task status:', err);
    }
}
