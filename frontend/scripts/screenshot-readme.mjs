#!/usr/bin/env node
/**
 * README screenshots — captures the AI features of Resume Pro.
 *
 * Prerequisites:
 *   - API on http://127.0.0.1:8080 (Redis running for the interview feature)
 *   - vite dev server on http://127.0.0.1:5173
 *   - seeded demo data: node scripts/seed-readme-demo.mjs
 *
 * Usage: node scripts/screenshot-readme.mjs
 * Output: docs/screenshots/*.png
 */

import { mkdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';

const __dirname = fileURLToPath(new URL('.', import.meta.url));
const frontendDir = resolve(__dirname, '..');
const outputDir = resolve(join(frontendDir, '..', 'docs', 'screenshots'));

const APP_URL = 'http://127.0.0.1:5173/';
const EMAIL = process.env.README_SHOT_EMAIL || 'readme-shot@example.com';
const PASSWORD = process.env.README_SHOT_PASSWORD || 'Readme-Shot-2026';
const RESUME_TITLE = process.env.README_SHOT_RESUME || 'Go 后端工程师简历';

const CHROME_PATHS = [
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const ANSWERS = {
  cache:
    '我们线上统一用 Cache-Aside：读请求先查缓存，miss 才回源数据库并回填；写请求先更新数据库，再删除缓存。'
    + '缓存删除失败会投递消息队列做补偿，再叠加延迟双删，把缓存与数据库的不一致窗口压到毫秒级。'
    + '缓存穿透用布隆过滤器加空值缓存挡掉；缓存击穿用互斥锁加逻辑过期，保证热点 key 只有一个请求回源；'
    + '缓存雪崩靠 TTL 随机化、多级本地缓存和 Redis Cluster 高可用来防。'
    + 'Redis Cluster 下按业务维度分片，热点 key 再用本地二级缓存做请求合并，TTL 短于 Redis 并通过失效订阅保证一致性。',
  ai:
    '一次玩家提问的完整链路是：先做意图识别，微调模型加规则兜底做混合路由，低置信度的问题直接转人工，避免答非所问；'
    + '识别为知识类问题后走 RAG，先向量召回再重排，把召回准确率从 54% 提升到 83%；'
    + '然后把召回内容和对话历史拼进 prompt 让模型生成答案，输出再做敏感词与合规校验。'
    + '转人工时把意图、命中的知识片段和对话摘要一并带过去，客服不需要重复追问。'
    + '效果上自动应答覆盖 68%，人工客服人力下降 70%。',
  mysql:
    '索引上用联合索引把最左匹配用满，把区分度低的字段放到最后，避免索引失效；分库分表按用户维度分片，流水表按时间分区。'
    + '慢查询靠慢日志和全链路 Trace 定位，把 P99 最高的接口逐个打掉；深分页改成游标方式，大促前用影子库压测确认容量。',
  idempotency:
    '幂等键我们用的是「用户 ID + 活动 ID + 道具 ID + 发放批次」拼成的业务唯一键，在发放入口先查幂等表，'
    + '命中就直接返回上一次的结果，不重复扣减。幂等表和发放记录放在同一个本地事务里写，靠唯一索引兜底，'
    + '并发插入冲突时捕获后回查，把幂等键生成、幂等表写入、道具流水落库做成一个事务。'
    + '重复请求返回结果如何处理上，我们把首次发放结果序列化后存进幂等表，重试直接回放这份结果，保证调用方拿到一致的成功响应。'
    + '服务拆分后跨服务发放的一致性，我们没有用分布式事务，而是走本地消息表加定时补偿：本地事务里同时写业务数据和消息表，'
    + '投递失败由补偿任务按幂等键重放，下游消费端也按幂等键去重，保证最终一致。',
  generic:
    '这个问题我按实际做法回答：先明确目标和约束，再给方案和取舍，最后用可观测数据验证效果。'
    + '我们高并发场景主要用 Redis 缓存加分库分表加限流降级这套组合，线上靠 Trace 和告警闭环，核心指标看 P99 和错误率。',
};

function answerFor(questionText) {
  const text = questionText || '';
  if (/Redis|缓存|Cache|穿透|击穿|雪崩|一致性/.test(text)) return ANSWERS.cache;
  if (/意图识别|知识库|客服|转人工|RAG|检索|向量/.test(text)) return ANSWERS.ai;
  if (/幂等|发放|唯一键|补偿|跨服务|事务原子/.test(text)) return ANSWERS.idempotency;
  if (/MySQL|数据库|索引|分库|慢查询/.test(text)) return ANSWERS.mysql;
  return ANSWERS.generic;
}

async function launch() {
  const executablePath = CHROME_PATHS.find((p) => existsSync(p));
  if (!executablePath) throw new Error('no chrome found');
  return puppeteer.launch({
    executablePath,
    headless: 'new',
    defaultViewport: { width: 1440, height: 900, deviceScaleFactor: 2 },
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });
}

async function clickByText(page, selector, text) {
  const handle = await page.evaluateHandle((sel, want) => {
    const nodes = Array.from(document.querySelectorAll(sel));
    return nodes.find((node) => (node.textContent || '').trim().includes(want)) || null;
  }, selector, text);
  const element = handle.asElement();
  if (!element) throw new Error('not found: ' + selector + ' "' + text + '"');
  await element.click();
  return element;
}

async function shot(page, name) {
  await page.screenshot({ path: join(outputDir, name) });
  console.log('shot:', name);
}

async function main() {
  await mkdir(outputDir, { recursive: true });
  const browser = await launch();
  const page = await browser.newPage();
  page.on('pageerror', (err) => console.log('[page error]', err.message.slice(0, 200)));
  page.on('console', (msg) => {
    if (msg.type() === 'error') console.log('[console error]', msg.text().slice(0, 200));
  });
  await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 2 });

  // 1. sign in
  await page.goto(APP_URL, { waitUntil: 'networkidle2' });
  await page.waitForSelector('input[type="email"]', { timeout: 20000 });
  await page.type('input[type="email"]', EMAIL);
  await page.type('input[type="password"]', PASSWORD);
  await clickByText(page, 'button', '登录');
  await page.waitForFunction(() => document.body.innerText.includes('模板主页'), { timeout: 30000 });
  console.log('signed in');

  // 2. template gallery
  await sleep(1200);
  await shot(page, '01-gallery.png');

  // 3. AI 快速生成 — the dialog is ready; the server-side generator is not wired yet.
  await clickByText(page, 'button', 'AI 快速生成');
  await page.waitForSelector('.ai-generate-modal textarea', { timeout: 10000 });
  await page.type(
    '.ai-generate-modal textarea',
    '5 年 Go 后端经验，主导过游戏运营后台服务化拆分与日均千万级请求的稳定性治理，'
      + '并负责 AI 客服从 0 到 1 落地，客服人力下降 70%。',
  );
  await sleep(600);
  await shot(page, '02-ai-quick-generate.png');
  await page.click('button[aria-label="关闭 AI 快速生成"]');
  await sleep(500);

  // 4. open the seeded resume in the editor
  await clickByText(page, 'nav button, button', '我的草稿');
  await page.waitForFunction((title) => document.body.innerText.includes(title), { timeout: 20000 }, RESUME_TITLE);
  await page.click('button[aria-label="继续编辑：' + RESUME_TITLE + '"]');
  await page.waitForSelector('.resume-workspace', { timeout: 15000 });
  await sleep(1500);
  await shot(page, '03-editor.png');

  // 5. AI 润色 — stream the rewrite suggestions for the summary section.
  await page.click('button[aria-label^="AI 润色"]');
  await page.waitForSelector('.ai-suggestion-panel', { timeout: 15000 });
  await page.waitForFunction(() => {
    const panel = document.querySelector('.ai-suggestion-panel');
    return panel && /建议已生成|改写建议|优化建议/.test(panel.innerText);
  }, { timeout: 240000 });
  await sleep(1200);
  await shot(page, '04-ai-polish.png');
  await page.click('button[aria-label="关闭 AI 润色建议"]');
  await sleep(500);

  // 6. the bound target job card inside the editor: JD preview + interview entry
  await page.waitForSelector('.target-job-card__interview', { timeout: 20000 });
  await page.evaluate(() => {
    const card = document.querySelector('.target-job-card');
    if (card) card.scrollIntoView({ block: 'center' });
  });
  await sleep(1200);
  await shot(page, '05-target-job-bound.png');

  // 7. interview configuration: 2 questions, no follow-ups, so the run is deterministic.
  await page.click('.target-job-card__interview');
  await page.waitForSelector('.interview-dialog', { timeout: 10000 });
  const step = async (label, times) => {
    for (let i = 0; i < times; i += 1) await page.click('button[aria-label="减少' + label + '"]');
  };
  await step('题目总数', 10);
  await step('八股题数量', 4);
  await step('算法题数量', 2);
  await step('项目题数量', 4);
  await step('追问总预算', 1);
  await step('单问追问上限', 1);
  await sleep(600);
  await shot(page, '06-interview-config.png');

  await clickByText(page, '.interview-dialog button', '开始面试');
  await page.waitForSelector('.interview-chat', { timeout: 240000 });
  await sleep(2000);
  await shot(page, '07-interview-chat.png');

  // 8. answer whatever is asked, then capture the graded turn and the review page
  for (let i = 0; i < 6; i += 1) {
    if (await page.$('.interview-review__body')) break;
    const question = await page.evaluate(() => {
      const node = document.querySelector('.interview-follow--current .interview-msg__question')
        || document.querySelector('.interview-msg--current .interview-msg__question');
      return node ? node.textContent : '';
    });
    console.log('answering:', question.slice(0, 40));
    await page.click('.interview-dock textarea');
    await page.type('.interview-dock textarea', answerFor(question));
    await clickByText(page, 'button', '提交回答');
    await page.waitForFunction(() => {
      if (document.querySelector('.interview-review__body')) return true;
      const area = document.querySelector('.interview-dock textarea');
      return Boolean(area) && area.value === '';
    }, { timeout: 300000 });
    await sleep(1500);
    if (i === 0) await shot(page, '08-interview-graded.png');
  }

  await page.waitForSelector('.interview-review__body', { timeout: 60000 });
  await sleep(1500);
  await shot(page, '09-interview-review.png');

  // 9. interview history — leave the review view first, the nav keeps the active interview.
  await clickByText(page, 'button', '返回列表');
  await sleep(1200);
  await clickByText(page, 'nav button, button', 'AI 面试');
  await sleep(1500);
  await shot(page, '10-interviews-list.png');

  // 10. target job list
  await clickByText(page, 'nav button, button', '目标岗位');
  await sleep(1500);
  await shot(page, '11-target-jobs.png');

  await browser.close();
  console.log('all shots done ->', outputDir);
}

main().catch((err) => {
  console.error('FAILED:', err.message);
  process.exit(1);
});
