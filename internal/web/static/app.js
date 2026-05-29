// Tab switching
document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
        document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
        tab.classList.add('active');
        document.getElementById(tab.dataset.tab).classList.add('active');

        // Load data when switching tabs
        if (tab.dataset.tab === 'status') loadStatus();
        if (tab.dataset.tab === 'feeds') loadFeeds();
        if (tab.dataset.tab === 'wiki') loadWiki();
        if (tab.dataset.tab === 'settings') loadSettings();
    });
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
                <button class="delete-btn" onclick="deleteFeed(${f.id})">删除</button>
            </div>
        `).join('');
    } catch (err) {
        console.error('Failed to load feeds:', err);
    }
}

async function addFeed() {
    const name = document.getElementById('feed-name').value.trim();
    const url = document.getElementById('feed-url').value.trim();
    const tags = document.getElementById('feed-tags').value.split(',').map(t => t.trim()).filter(Boolean);

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

        document.getElementById('feed-name').value = '';
        document.getElementById('feed-url').value = '';
        document.getElementById('feed-tags').value = '';
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
        list.innerHTML = pages.map(p => `
            <div class="wiki-item" onclick="loadWikiPage('${p.slug}')">
                <h3>${escapeHtml(p.title)}</h3>
                <div class="tags">
                    ${p.tags.map(t => `<span class="tag">${escapeHtml(t)}</span>`).join('')}
                </div>
            </div>
        `).join('');

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

// Settings
async function loadSettings() {
    try {
        const res = await fetch('/api/settings');
        const settings = await res.json();

        // LLM
        if (settings.llm) {
            document.getElementById('llm-model').value = settings.llm.model || '';
            document.getElementById('llm-base-url').value = settings.llm.base_url || '';
            // Don't set api_key value for security
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
                    model: document.getElementById('dedup-vector-model').value
                }
            };
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

// Keyboard shortcut
document.getElementById('question').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        askQuestion();
    }
});
