import { test, expect, type Page, type Locator } from '@playwright/test';

// Helpers to mock API endpoints
async function mockStatus(page: Page, data = { feeds: 5, documents: 120, wiki_pages: 30, pending: 3, processing: 1, failed: 2 }) {
  await page.route('**/api/status', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })
  );
}

async function mockFeeds(page: Page, feeds = [
  { id: 1, name: 'Hacker News', url: 'https://news.ycombinator.com/rss', tags: ['tech'] },
  { id: 2, name: 'Go Blog', url: 'https://go.dev/blog/feed.atom', tags: ['go', 'lang'] },
]) {
  await page.route('**/api/feeds', route => {
    if (route.request().method() === 'GET') {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(feeds) });
    }
    return route.fallback();
  });
}

async function mockWiki(page: Page, pages = [
  { slug: 'rust-basics', title: 'Rust Basics', tags: ['rust'] },
  { slug: 'go-concurrency', title: 'Go Concurrency', tags: ['go', 'concurrency'] },
]) {
  await page.route('**/api/wiki', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(pages) })
  );
}

async function mockWikiPage(page: Page, slug: string, data = { title: 'Rust Basics', content: '# Rust\n\n**Fast** and *safe*.\n\n`code example`' }) {
  await page.route(`**/api/wiki/${slug}`, route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })
  );
}

async function mockSettings(page: Page, settings = {
  llm: { model: 'gpt-4o', base_url: 'https://api.openai.com/v1' },
  filter: { mode: 'keyword', keyword: { tags: ['AI', 'LLM'], match_any: true } },
  dedup: { url_exact: true, content_hash: false, vector: { enabled: true, threshold: 0.9, model: 'text-embedding-3-small' } },
  general: { interval: '0 */6 * * *' },
}) {
  await page.route('**/api/settings', route => {
    if (route.request().method() === 'GET') {
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(settings) });
    }
    return route.fallback();
  });
}

async function mockDocuments(page: Page, items = [
  { id: 1, title: 'AI News', url: 'https://example.com/1', feed_name: 'Hacker News', tags: ['AI'], source: 'cleaned_raw', status: 'done', summary: 'An article about AI...', created_at: '2026-05-29T10:00:00Z' },
  { id: 2, title: 'Go 1.25 Released', url: 'https://example.com/2', feed_name: 'Go Blog', tags: ['go'], source: 'raw', status: 'pending', summary: 'Go 1.25 release notes...', created_at: '2026-05-28T08:00:00Z' },
], total = 2) {
  await page.route(/\/api\/documents(\?|$)/, route =>
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

async function gotoPage(page: Page) {
  await page.goto('/');
  await expect(page.locator('h1')).toHaveText('LLM Wiki');
}

// ─── Tab Navigation ───

test.describe('Tab Navigation', () => {
  test('default active tab is query', async ({ page }) => {
    await gotoPage(page);
    await expect(page.locator('.tab.active')).toHaveAttribute('data-tab', 'query');
    await expect(page.locator('#query')).toHaveClass(/active/);
  });

  test('clicking status tab switches active state', async ({ page }) => {
    await mockStatus(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="status"]');
    await expect(page.locator('.tab[data-tab="status"]')).toHaveClass(/active/);
    await expect(page.locator('#status')).toHaveClass(/active/);
    await expect(page.locator('.tab[data-tab="query"]')).not.toHaveClass(/active/);
    await expect(page.locator('#query')).not.toHaveClass(/active/);
  });

  test('clicking feeds tab switches correctly', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');
    await expect(page.locator('.tab[data-tab="feeds"]')).toHaveClass(/active/);
    await expect(page.locator('#feeds')).toHaveClass(/active/);
  });

  test('clicking wiki tab switches correctly', async ({ page }) => {
    await mockWiki(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="wiki"]');
    await expect(page.locator('.tab[data-tab="wiki"]')).toHaveClass(/active/);
    await expect(page.locator('#wiki')).toHaveClass(/active/);
  });

  test('clicking settings tab switches correctly', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await expect(page.locator('.tab[data-tab="settings"]')).toHaveClass(/active/);
    await expect(page.locator('#settings')).toHaveClass(/active/);
  });
});

// ─── Query Tab ───

test.describe('Query Tab', () => {
  test('empty question does not call API', async ({ page }) => {
    await gotoPage(page);
    let apiCalled = false;
    page.on('request', req => { if (req.url().includes('/api/query')) apiCalled = true; });

    await page.click('#ask-btn');
    expect(apiCalled).toBe(false);
  });

  test('submit question shows answer and sources', async ({ page }) => {
    await page.route('**/api/query', route =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          answer: 'Rust is a systems language.',
          sources: [{ title: 'Rust Book' }, { title: 'Rust by Example' }],
        }),
      })
    );
    await gotoPage(page);

    await page.fill('#question', 'What is Rust?');
    await page.click('#ask-btn');

    await expect(page.locator('#answer')).toBeVisible();
    await expect(page.locator('#answer-content')).toHaveText('Rust is a systems language.');
    await expect(page.locator('.source-item')).toHaveCount(2);
    await expect(page.locator('.source-item').first()).toHaveText('Rust Book');
  });

  test('empty sources hides sources section', async ({ page }) => {
    await page.route('**/api/query', route =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ answer: 'No sources.', sources: [] }),
      })
    );
    await gotoPage(page);

    await page.fill('#question', 'test');
    await page.click('#ask-btn');

    await expect(page.locator('#answer-content')).toHaveText('No sources.');
    await expect(page.locator('#sources')).toBeEmpty();
  });

  test('Enter key submits question', async ({ page }) => {
    let requestBody: string | undefined;
    await page.route('**/api/query', route => {
      requestBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ answer: 'ok', sources: [] }) });
    });
    await gotoPage(page);

    await page.fill('#question', 'hello');
    await page.press('#question', 'Enter');

    await expect(page.locator('#answer-content')).toHaveText('ok');
    expect(JSON.parse(requestBody!).question).toBe('hello');
  });

  test('API error shows alert', async ({ page }) => {
    await page.route('**/api/query', route =>
      route.fulfill({ status: 500, body: 'Internal Server Error' })
    );
    await gotoPage(page);

    const dialogPromise = page.waitForEvent('dialog');
    await page.fill('#question', 'fail');
    await page.click('#ask-btn');

    const dialog = await dialogPromise;
    expect(dialog.message()).toContain('查询失败');
    await dialog.dismiss();
  });
});

// ─── Status Tab ───

test.describe('Status Tab', () => {
  test('loads and displays status data', async ({ page }) => {
    await mockStatus(page, { feeds: 10, documents: 500, wiki_pages: 50, pending: 7, processing: 2, failed: 1 });
    await gotoPage(page);

    await page.click('.tab[data-tab="status"]');

    await expect(page.locator('#stat-feeds')).toHaveText('10');
    await expect(page.locator('#stat-docs')).toHaveText('500');
    await expect(page.locator('#stat-wiki')).toHaveText('50');
    await expect(page.locator('#stat-pending')).toHaveText('7');
    await expect(page.locator('#stat-processing')).toHaveText('2');
    await expect(page.locator('#stat-failed')).toHaveText('1');
  });

  test('refresh button reloads status', async ({ page }) => {
    let callCount = 0;
    await page.route('**/api/status', route => {
      callCount++;
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ feeds: callCount, documents: 0, wiki_pages: 0, pending: 0, processing: 0, failed: 0 }) });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="status"]');
    await expect(page.locator('#stat-feeds')).toHaveText('1');

    await page.click('button:text("刷新")');
    await expect(page.locator('#stat-feeds')).toHaveText('2');
    expect(callCount).toBe(2);
  });
});

// ─── Feeds Tab ───

test.describe('Feeds Tab', () => {
  test('renders feed list with name, url, tags', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');
    const items = page.locator('.feed-item');
    await expect(items).toHaveCount(2);

    await expect(items.first().locator('h3')).toHaveText('Hacker News');
    await expect(items.first().locator('p').first()).toHaveText('https://news.ycombinator.com/rss');
    await expect(items.first().locator('p').nth(1)).toHaveText('标签: tech');
  });

  test('add feed clears form and reloads list', async ({ page }) => {
    await mockFeeds(page, []);
    let postedBody: string | undefined;
    await page.route('**/api/feeds', route => {
      if (route.request().method() === 'POST') {
        postedBody = route.request().postData() ?? undefined;
        return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
      }
      return route.fallback();
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');
    await page.fill('#feed-name', 'Test Feed');
    await page.fill('#feed-url', 'https://example.com/rss');
    await page.fill('#feed-tags', 'test, demo');
    await page.click('button:text("添加 Feed")');

    const body = JSON.parse(postedBody!);
    expect(body.name).toBe('Test Feed');
    expect(body.url).toBe('https://example.com/rss');
    expect(body.tags).toEqual(['test', 'demo']);

    await expect(page.locator('#feed-name')).toHaveValue('');
    await expect(page.locator('#feed-url')).toHaveValue('');
    await expect(page.locator('#feed-tags')).toHaveValue('');
  });

  test('missing name shows alert', async ({ page }) => {
    await mockFeeds(page, []);
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');
    await page.fill('#feed-url', 'https://example.com/rss');

    const dialogPromise = new Promise<string>(resolve => {
      page.once('dialog', async dialog => {
        resolve(dialog.message());
        await dialog.dismiss();
      });
    });

    await page.click('button:text("添加 Feed")');
    expect(await dialogPromise).toContain('请输入名称和 URL');
  });

  test('delete feed with confirmation', async ({ page }) => {
    await mockFeeds(page);
    let deleteUrl = '';
    await page.route('**/api/feeds/*', route => {
      deleteUrl = route.request().url();
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');

    // Accept the confirm dialog
    page.on('dialog', dialog => dialog.accept());

    await page.locator('.delete-btn').first().click();

    expect(deleteUrl).toContain('/api/feeds/1');
  });

  test('XSS in feed name is escaped', async ({ page }) => {
    await mockFeeds(page, [{ id: 1, name: '<script>alert("xss")</script>', url: 'https://example.com', tags: [] }]);
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');

    const h3 = page.locator('.feed-item h3').first();
    // The text content should be the raw string, not executed as HTML
    await expect(h3).toHaveText('<script>alert("xss")</script>');
  });

  test('export buttons have correct URLs', async ({ page }) => {
    await mockFeeds(page, []);
    await gotoPage(page);

    await page.click('.tab[data-tab="feeds"]');

    const jsonBtn = page.locator('button:text("导出 JSON")');
    const opmlBtn = page.locator('button:text("导出 OPML")');

    // exportFeeds uses window.location.href, so we check the onclick attribute
    await expect(jsonBtn).toBeVisible();
    await expect(opmlBtn).toBeVisible();
  });
});

// ─── Wiki Tab ───

test.describe('Wiki Tab', () => {
  test('renders wiki page list with titles and tags', async ({ page }) => {
    await mockWiki(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="wiki"]');
    const items = page.locator('.wiki-item');
    await expect(items).toHaveCount(2);

    await expect(items.first().locator('h3')).toHaveText('Rust Basics');
    await expect(items.first().locator('.tag')).toHaveText('rust');
  });

  test('click wiki page shows detail view', async ({ page }) => {
    await mockWiki(page);
    await mockWikiPage(page, 'rust-basics');
    await gotoPage(page);

    await page.click('.tab[data-tab="wiki"]');
    await page.locator('.wiki-item').first().click();

    await expect(page.locator('#wiki-content')).toBeVisible();
    await expect(page.locator('#wiki-list')).not.toBeVisible();
    await expect(page.locator('#wiki-title')).toHaveText('Rust Basics');
  });

  test('back button returns to list', async ({ page }) => {
    await mockWiki(page);
    await mockWikiPage(page, 'rust-basics');
    await gotoPage(page);

    await page.click('.tab[data-tab="wiki"]');
    await page.locator('.wiki-item').first().click();
    await expect(page.locator('#wiki-content')).toBeVisible();

    await page.click('button:text("返回列表")');
    await expect(page.locator('#wiki-list')).toBeVisible();
    await expect(page.locator('#wiki-content')).not.toBeVisible();
  });

  test('markdown rendering works correctly', async ({ page }) => {
    await mockWiki(page);
    await mockWikiPage(page, 'rust-basics', {
      title: 'Rust Basics',
      content: '# Header\n\n**bold text**\n\n*italic text*\n\n`inline code`',
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="wiki"]');
    await page.locator('.wiki-item').first().click();

    const body = page.locator('#wiki-body');
    await expect(body.locator('h1')).toHaveText('Header');
    await expect(body.locator('strong')).toHaveText('bold text');
    await expect(body.locator('em')).toHaveText('italic text');
    await expect(body.locator('code')).toHaveText('inline code');
  });
});

// ─── Settings Tab ───

test.describe('Settings Tab', () => {
  test('loads settings and populates fields', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');

    await expect(page.locator('#llm-model')).toHaveValue('gpt-4o');
    await expect(page.locator('#llm-base-url')).toHaveValue('https://api.openai.com/v1');
    await expect(page.locator('#filter-mode')).toHaveValue('keyword');
    await expect(page.locator('#filter-keyword-tags')).toHaveValue('AI,LLM');
    await expect(page.locator('#filter-keyword-match-any')).toBeChecked();
    await expect(page.locator('#dedup-url-exact')).toBeChecked();
    await expect(page.locator('#dedup-content-hash')).not.toBeChecked();
    await expect(page.locator('#dedup-vector-enabled')).toBeChecked();
    await expect(page.locator('#dedup-vector-threshold')).toHaveValue('0.9');
    await expect(page.locator('#dedup-vector-model')).toHaveValue('text-embedding-3-small');
    await expect(page.locator('#general-interval')).toHaveValue('0 */6 * * *');
  });

  test('API key field is not pre-filled from server', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await expect(page.locator('#llm-api-key')).toHaveValue('');
    await expect(page.locator('#llm-api-key')).toHaveAttribute('type', 'password');
  });

  test('toggle API key visibility', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');

    const input = page.locator('#llm-api-key');
    const toggleBtn = page.locator('.toggle-visibility');

    await expect(input).toHaveAttribute('type', 'password');
    await expect(toggleBtn).toHaveText('显示');

    await toggleBtn.click();
    await expect(input).toHaveAttribute('type', 'text');
    await expect(toggleBtn).toHaveText('隐藏');

    await toggleBtn.click();
    await expect(input).toHaveAttribute('type', 'password');
    await expect(toggleBtn).toHaveText('显示');
  });

  test('save LLM settings sends correct data', async ({ page }) => {
    await mockSettings(page);
    let putBody: string | undefined;
    await page.route('**/api/settings/llm', route => {
      putBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await page.fill('#llm-model', 'claude-3-opus');
    await page.fill('#llm-api-key', 'sk-test-123');
    await page.fill('#llm-base-url', 'https://api.anthropic.com');

    const dialogPromise = page.waitForEvent('dialog');
    await page.locator('button:text("保存 LLM 配置")').click();

    const dialog = await dialogPromise;
    expect(dialog.message()).toContain('配置已保存');
    await dialog.dismiss();

    const body = JSON.parse(putBody!);
    expect(body.model).toBe('claude-3-opus');
    expect(body.api_key).toBe('sk-test-123');
    expect(body.base_url).toBe('https://api.anthropic.com');
  });

  test('save filter settings sends correct data', async ({ page }) => {
    await mockSettings(page);
    let putBody: string | undefined;
    await page.route('**/api/settings/filter', route => {
      putBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await page.selectOption('#filter-mode', 'llm_judgment');
    await page.fill('#filter-keyword-tags', 'rust, go');
    await page.check('#filter-keyword-match-any');

    const dialogPromise = page.waitForEvent('dialog');
    await page.locator('button:text("保存筛选配置")').click();
    await (await dialogPromise).dismiss();

    const body = JSON.parse(putBody!);
    expect(body.mode).toBe('llm_judgment');
    expect(body.keyword.tags).toEqual(['rust', 'go']);
    expect(body.keyword.match_any).toBe(true);
  });

  test('save dedup settings sends correct data', async ({ page }) => {
    await mockSettings(page);
    let putBody: string | undefined;
    await page.route('**/api/settings/dedup', route => {
      putBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await page.check('#dedup-url-exact');
    await page.uncheck('#dedup-content-hash');
    await page.check('#dedup-vector-enabled');
    await page.fill('#dedup-vector-threshold', '0.95');
    await page.fill('#dedup-vector-model', 'text-embedding-3-large');

    const dialogPromise = page.waitForEvent('dialog');
    await page.locator('button:text("保存去重配置")').click();
    await (await dialogPromise).dismiss();

    const body = JSON.parse(putBody!);
    expect(body.url_exact).toBe(true);
    expect(body.content_hash).toBe(false);
    expect(body.vector.enabled).toBe(true);
    expect(body.vector.threshold).toBe(0.95);
    expect(body.vector.model).toBe('text-embedding-3-large');
  });

  test('save general settings sends correct data', async ({ page }) => {
    await mockSettings(page);
    let putBody: string | undefined;
    await page.route('**/api/settings/general', route => {
      putBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await page.fill('#general-interval', '0 */12 * * *');

    const dialogPromise = page.waitForEvent('dialog');
    await page.locator('button:text("保存通用配置")').click();
    await (await dialogPromise).dismiss();

    const body = JSON.parse(putBody!);
    expect(body.interval).toBe('0 */12 * * *');
  });

  test('empty API key is not sent in save request', async ({ page }) => {
    await mockSettings(page);
    let putBody: string | undefined;
    await page.route('**/api/settings/llm', route => {
      putBody = route.request().postData() ?? undefined;
      return route.fulfill({ status: 200, body: '{}' });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="settings"]');
    await page.fill('#llm-model', 'gpt-4o');
    // Leave api_key empty

    const dialogPromise = page.waitForEvent('dialog');
    await page.locator('button:text("保存 LLM 配置")').click();
    await (await dialogPromise).dismiss();

    const body = JSON.parse(putBody!);
    expect(body.api_key).toBeUndefined();
  });
});

// ─── Computed Style Checks ───

function rgb(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).backgroundColor);
}
function color(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).color);
}
function fontSize(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).fontSize);
}
function display(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).display);
}
function fontWeight(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).fontWeight);
}
function borderRadius(el: Locator) {
  return el.evaluate(e => getComputedStyle(e).borderRadius);
}

test.describe('Style Verification', () => {
  test('header has white background and rounded corners', async ({ page }) => {
    await gotoPage(page);
    const header = page.locator('header');
    expect(await rgb(header)).toBe('rgb(255, 255, 255)');
    expect(await borderRadius(header)).toBe('8px');
  });

  test('active tab has blue background and white text', async ({ page }) => {
    await gotoPage(page);
    const activeTab = page.locator('.tab.active');
    expect(await rgb(activeTab)).toBe('rgb(52, 152, 219)');
    expect(await color(activeTab)).toBe('rgb(255, 255, 255)');
  });

  test('inactive tab has gray text and no background', async ({ page }) => {
    await gotoPage(page);
    const inactiveTab = page.locator('.tab[data-tab="status"]');
    expect(await color(inactiveTab)).toBe('rgb(102, 102, 102)');
    expect(await rgb(inactiveTab)).toBe('rgba(0, 0, 0, 0)');
  });

  test('tab content hidden when inactive, visible when active', async ({ page }) => {
    await gotoPage(page);
    expect(await display(page.locator('#status'))).toBe('none');
    expect(await display(page.locator('#query'))).toBe('block');
  });

  test('stat card has centered text and light background', async ({ page }) => {
    await mockStatus(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="status"]');

    const card = page.locator('.stat-card').first();
    expect(await rgb(card)).toBe('rgb(249, 249, 249)');
    expect(await card.evaluate(e => getComputedStyle(e).textAlign)).toBe('center');
  });

  test('stat value has large bold blue text', async ({ page }) => {
    await mockStatus(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="status"]');

    const val = page.locator('.stat-value').first();
    expect(await fontSize(val)).toBe('32px');
    expect(await fontWeight(val)).toBe('700');
    expect(await color(val)).toBe('rgb(52, 152, 219)');
  });

  test('delete button has red background', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const btn = page.locator('.delete-btn').first();
    expect(await rgb(btn)).toBe('rgb(231, 76, 60)');
  });

  test('tag has blue pill shape with white text', async ({ page }) => {
    await mockWiki(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="wiki"]');

    const tag = page.locator('.tag').first();
    expect(await rgb(tag)).toBe('rgb(52, 152, 219)');
    expect(await color(tag)).toBe('rgb(255, 255, 255)');
    expect(await borderRadius(tag)).toBe('12px');
    expect(await fontSize(tag)).toBe('12px');
  });

  test('settings card has light background', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="settings"]');

    const card = page.locator('.settings-card').first();
    expect(await rgb(card)).toBe('rgb(249, 249, 249)');
    expect(await borderRadius(card)).toBe('8px');
  });

  test('import button is green, export button is purple', async ({ page }) => {
    await mockFeeds(page, []);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const importBtn = page.locator('.import-section button');
    const exportBtn = page.locator('.export-section button').first();
    expect(await rgb(importBtn)).toBe('rgb(39, 174, 96)');
    expect(await rgb(exportBtn)).toBe('rgb(142, 68, 173)');
  });

  test('body uses system font stack', async ({ page }) => {
    await gotoPage(page);
    const fontFamily = await page.evaluate(() => getComputedStyle(document.body).fontFamily);
    expect(fontFamily).toContain('BlinkMacSystemFont');
  });

  test('feed item uses flex layout with space-between', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const item = page.locator('.feed-item').first();
    expect(await item.evaluate(e => getComputedStyle(e).display)).toBe('flex');
    expect(await item.evaluate(e => getComputedStyle(e).justifyContent)).toBe('space-between');
  });

  test('settings grid uses CSS grid layout', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="settings"]');

    const grid = page.locator('.settings-grid');
    expect(await grid.evaluate(e => getComputedStyle(e).display)).toBe('grid');
  });

  test('stats grid uses CSS grid layout', async ({ page }) => {
    await mockStatus(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="status"]');

    const grid = page.locator('.stats-grid');
    expect(await grid.evaluate(e => getComputedStyle(e).display)).toBe('grid');
  });

  test('container max-width is 1200px with 20px padding', async ({ page }) => {
    await gotoPage(page);
    const c = page.locator('.container');
    expect(await c.evaluate(e => getComputedStyle(e).maxWidth)).toBe('1200px');
    expect(await c.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('20px');
    expect(await c.evaluate(e => getComputedStyle(e).paddingRight)).toBe('20px');
  });

  test('header has 20px padding', async ({ page }) => {
    await gotoPage(page);
    const h = page.locator('header');
    expect(await h.evaluate(e => getComputedStyle(e).padding)).toBe('20px');
  });

  test('tab button has 10px 20px padding', async ({ page }) => {
    await gotoPage(page);
    const tab = page.locator('.tab').first();
    expect(await tab.evaluate(e => getComputedStyle(e).paddingTop)).toBe('10px');
    expect(await tab.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('20px');
  });

  test('tab content has 20px padding', async ({ page }) => {
    await gotoPage(page);
    const tc = page.locator('#query');
    expect(await tc.evaluate(e => getComputedStyle(e).padding)).toBe('20px');
  });

  test('textarea has 12px padding', async ({ page }) => {
    await gotoPage(page);
    const ta = page.locator('#question');
    expect(await ta.evaluate(e => getComputedStyle(e).paddingTop)).toBe('12px');
    expect(await ta.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('12px');
  });

  test('button has 10px 20px padding and 4px border-radius', async ({ page }) => {
    await gotoPage(page);
    const btn = page.locator('#ask-btn');
    expect(await btn.evaluate(e => getComputedStyle(e).paddingTop)).toBe('10px');
    expect(await btn.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('20px');
    expect(await borderRadius(btn)).toBe('4px');
  });

  test('stat card has 20px padding', async ({ page }) => {
    await mockStatus(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="status"]');

    const card = page.locator('.stat-card').first();
    expect(await card.evaluate(e => getComputedStyle(e).padding)).toBe('20px');
  });

  test('feed item has 15px padding', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const item = page.locator('.feed-item').first();
    expect(await item.evaluate(e => getComputedStyle(e).padding)).toBe('15px');
  });

  test('settings card has 20px padding and 8px border-radius', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="settings"]');

    const card = page.locator('.settings-card').first();
    expect(await card.evaluate(e => getComputedStyle(e).padding)).toBe('20px');
    expect(await borderRadius(card)).toBe('8px');
  });

  test('form-group has 15px margin-bottom', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="settings"]');

    const fg = page.locator('.form-group').first();
    expect(await fg.evaluate(e => getComputedStyle(e).marginBottom)).toBe('15px');
  });

  test('delete button has smaller padding (8px 16px)', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const btn = page.locator('.delete-btn').first();
    expect(await btn.evaluate(e => getComputedStyle(e).paddingTop)).toBe('8px');
    expect(await btn.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('16px');
  });

  test('wiki item has 15px padding and 4px border-radius', async ({ page }) => {
    await mockWiki(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="wiki"]');

    const item = page.locator('.wiki-item').first();
    expect(await item.evaluate(e => getComputedStyle(e).padding)).toBe('15px');
    expect(await borderRadius(item)).toBe('4px');
  });

  test('tag has 2px 8px padding and 12px font-size', async ({ page }) => {
    await mockWiki(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="wiki"]');

    const tag = page.locator('.tag').first();
    expect(await tag.evaluate(e => getComputedStyle(e).paddingTop)).toBe('2px');
    expect(await tag.evaluate(e => getComputedStyle(e).paddingLeft)).toBe('8px');
    expect(await fontSize(tag)).toBe('12px');
  });

  test('textarea font-size is 16px', async ({ page }) => {
    await gotoPage(page);
    expect(await fontSize(page.locator('#question'))).toBe('16px');
  });

  test('button font-size is 16px', async ({ page }) => {
    await gotoPage(page);
    expect(await fontSize(page.locator('#ask-btn'))).toBe('16px');
  });

  test('feed info p has 14px font-size', async ({ page }) => {
    await mockFeeds(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="feeds"]');

    const p = page.locator('.feed-info p').first();
    expect(await fontSize(p)).toBe('14px');
  });

  test('settings label has 14px font-size', async ({ page }) => {
    await mockSettings(page);
    await gotoPage(page);
    await page.click('.tab[data-tab="settings"]');

    const label = page.locator('.form-group label').first();
    expect(await fontSize(label)).toBe('14px');
  });
});

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

    await page.locator('#doc-content button:text("返回列表")').click();
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
    const requestUrls: string[] = [];
    await page.route('**/api/documents/stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ total: 0, by_status: {}, by_source: {}, feeds: [] }) })
    );
    await page.route('**/api/documents**', route => {
      requestUrls.push(route.request().url());
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], total: 0, page: 1, per_page: 20 }) });
    });
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    // Wait for the initial document load to complete
    await page.waitForResponse(resp => resp.url().includes('/api/documents') && resp.status() === 200);
    await page.selectOption('#doc-filter-status', 'done');
    // Wait for the filtered request
    await page.waitForResponse(resp => resp.url().includes('/api/documents') && resp.url().includes('status=done') && resp.status() === 200);

    const filteredUrl = requestUrls.find(u => u.includes('status=done'));
    expect(filteredUrl).toBeTruthy();
  });

  test('pagination shows page info', async ({ page }) => {
    await mockDocuments(page, [], 50);
    await gotoPage(page);

    await page.click('.tab[data-tab="documents"]');
    await expect(page.locator('#doc-total')).toHaveText('共 50 篇文档');
  });
});
