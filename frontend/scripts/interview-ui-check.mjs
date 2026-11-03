#!/usr/bin/env node
/**
 * Audits the in-progress interview chat at desktop and phone widths against an
 * existing active interview, so no question generation is triggered.
 *
 * Usage: INTERVIEW_UI_EMAIL=… INTERVIEW_UI_PASSWORD=… node scripts/interview-ui-check.mjs [outputDir]
 * Requires the vite dev server on 5173 and the API on 8080.
 */

import { mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';

const __dirname = fileURLToPath(new URL('.', import.meta.url));
const frontendDir = resolve(__dirname, '..');
const outputDir = resolve(process.argv[2] || join(frontendDir, '..', '.tmp-interview-ui'));

const APP_URL = 'http://127.0.0.1:5173/interviews';
const CHROME_PATHS = [
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
];

const EMAIL = process.env.INTERVIEW_UI_EMAIL || '';
const PASSWORD = process.env.INTERVIEW_UI_PASSWORD || '';
if (!EMAIL || !PASSWORD) {
  console.error('set INTERVIEW_UI_EMAIL and INTERVIEW_UI_PASSWORD');
  process.exit(1);
}

const VIEWPORTS = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'phone', width: 390, height: 844 },
];

async function launch() {
  const { existsSync } = await import('node:fs');
  const executablePath = CHROME_PATHS.find((path) => existsSync(path));
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
    return nodes.find((node) => (node.textContent || '').includes(want)) || null;
  }, selector, text);
  const element = handle.asElement();
  if (!element) throw new Error('not found: ' + selector + ' ' + text);
  await element.click();
  return element;
}

function measure() {
  const rect = (sel) => {
    const node = document.querySelector(sel);
    if (!node) return null;
    const box = node.getBoundingClientRect();
    return { w: Math.round(box.width), right: Math.round(box.right), top: Math.round(box.top), bottom: Math.round(box.bottom) };
  };
  const overflow = [];
  for (const node of document.querySelectorAll('.interview-chat *')) {
    const box = node.getBoundingClientRect();
    if (box.right > window.innerWidth + 1 || box.left < -1) {
      overflow.push({ cls: node.className.toString().slice(0, 60), right: Math.round(box.right), w: Math.round(box.width) });
    }
  }
  const dock = document.querySelector('.interview-dock');
  return {
    innerWidth: window.innerWidth,
    scrollWidth: document.scrollingElement.scrollWidth,
    chat: rect('.interview-chat'),
    progress: rect('.interview-progress'),
    progressSticky: dock ? getComputedStyle(document.querySelector('.interview-progress')).position : null,
    dock: rect('.interview-dock'),
    dockSticky: dock ? getComputedStyle(dock).position : null,
    segments: document.querySelectorAll('.interview-progress__seg').length,
    currentSegment: (document.querySelector('.interview-progress__seg--current') || {}).textContent,
    turns: document.querySelectorAll('.interview-chat__stream .interview-msg').length,
    pastTurns: document.querySelectorAll('.interview-msg--past').length,
    currentTurns: document.querySelectorAll('.interview-msg--current').length,
    followUps: document.querySelectorAll('.interview-follow').length,
    heading: (document.querySelector('.interview-progress__count') || {}).textContent,
    stats: Array.from(document.querySelectorAll('.interview-stat')).map((n) => n.textContent.trim()),
    overflow: overflow.slice(0, 6),
  };
}

async function main() {
  await mkdir(outputDir, { recursive: true });
  const browser = await launch();

  for (const viewport of VIEWPORTS) {
    const page = await browser.newPage();
    page.on('pageerror', (err) => console.log('[page error]', err.message.slice(0, 200)));
    page.on('console', (msg) => {
      if (msg.type() === 'error') console.log('[console error]', msg.text().slice(0, 200));
    });
    await page.setViewport({ ...viewport, deviceScaleFactor: 2 });

    await page.goto(APP_URL, { waitUntil: 'networkidle2' });
    const onAuthScreen = await page.evaluate(() => Boolean(document.querySelector('input[type="email"], input[type="text"]')));
    if (onAuthScreen) {
      const inputs = await page.$$('input');
      await inputs[0].type(EMAIL);
      await inputs[1].type(PASSWORD);
      await clickByText(page, 'button', '登录');
      await page.waitForFunction(() => document.body.innerText.includes('模板主页'), { timeout: 20000 });
    }

    // Navigate in-app: the gallery redirect can overwrite a direct /interviews load.
    await clickByText(page, 'nav button, button', 'AI 面试');
    await new Promise((r) => setTimeout(r, 800));
    await page.screenshot({ path: join(outputDir, `${viewport.name}-1-list.png`) });

    // An active interview shows 继续面试; otherwise the flow starts from a resume editor.
    const resumeButton = await page.evaluateHandle(() =>
      Array.from(document.querySelectorAll('button')).find((n) => (n.textContent || '').includes('继续面试')) || null);
    if (!resumeButton.asElement()) {
      const dump = await page.evaluate(() => ({ url: location.href, text: document.body.innerText.slice(0, 800), buttons: Array.from(document.querySelectorAll('button')).map((n) => n.textContent.trim()) }));
      throw new Error('no 继续面试 button: ' + JSON.stringify(dump));
    }
    await resumeButton.asElement().click();
    await page.waitForSelector('.interview-chat', { timeout: 15000 });
    await new Promise((r) => setTimeout(r, 1200));
    await page.screenshot({ path: join(outputDir, `${viewport.name}-2-chat.png`) });

    const state = await page.evaluate(measure);
    console.log('=== ' + viewport.name + ' (' + viewport.width + 'x' + viewport.height + ')');
    console.log(JSON.stringify(state, null, 2));

    // Answering is not part of this audit; type locally to measure the composer only.
    const area = await page.$('.interview-dock textarea');
    if (!area) throw new Error('no composer textarea');
    const before = await page.evaluate(() => {
      const node = document.querySelector('.interview-dock textarea');
      return { placeholder: node.placeholder, height: node.offsetHeight, overflow: getComputedStyle(node).overflowY };
    });
    await area.click();
    await area.type('在 mycache 的实现里，我用了分片锁把热点 key 的争用拆开，淘汰策略选了 W-TinyLFU。这里的问题是主线程在等待 hchan 的 sendx 时，sudog 会被挂到 sendq 上并调用 gopark，随后由 recvx 唤醒。缓冲环形队列满了以后，写入方会阻塞并记录 producer 视角的背压，等消费方推进 tail 指针才有机会继续。这个方案后来在压测里暴露出锁粒度过粗，我又补了一层本地缓存来降低跨核失效。');
    await new Promise((r) => setTimeout(r, 400));
    const composer = await page.evaluate((start) => {
      const node = document.querySelector('.interview-dock textarea');
      const dock = document.querySelector('.interview-dock');
      const styles = getComputedStyle(node);
      const dockStyles = getComputedStyle(dock);
      const viewportBottom = window.innerHeight;
      return {
        placeholder: start.placeholder,
        heightBefore: start.height,
        focused: document.activeElement === node,
        height: node.offsetHeight,
        scrollHeight: node.scrollHeight,
        maxHeight: styles.maxHeight,
        resize: styles.resize,
        outline: styles.outlineWidth + ' ' + styles.outlineStyle,
        dockBoxShadow: dockStyles.boxShadow,
        dockBorder: dockStyles.borderColor,
        dockBottom: Math.round(dock.getBoundingClientRect().bottom),
        viewportBottom: Math.round(viewportBottom),
        internalScroll: node.scrollHeight > node.clientHeight,
        standardAnswerLeak: document.body.innerText.includes('查看标准答案'),
        pointsLeak: document.body.innerText.includes('必答点'),
      };
    }, before);
    console.log('dock:', JSON.stringify(composer, null, 2));
    await page.screenshot({ path: join(outputDir, `${viewport.name}-3-composer.png`) });

    // The report card is read-only; capture it at desktop width only.
    if (viewport.name === 'desktop') {
      try {
        await clickByText(page, 'button', '返回列表');
        await new Promise((r) => setTimeout(r, 600));
        await clickByText(page, 'button', '查看报告');
        await page.waitForSelector('.interview-card__review', { timeout: 10000 });
        await new Promise((r) => setTimeout(r, 500));
        const review = await page.evaluate(() => {
          const node = document.querySelector('.interview-card__review');
          const body = node.innerText;
          return {
            html: node.innerHTML.slice(0, 500),
            chars: body.length,
            lists: node.querySelectorAll('ul, ol').length,
            items: node.querySelectorAll('li').length,
            bold: node.querySelectorAll('strong').length,
            whiteSpace: getComputedStyle(node).whiteSpace,
          };
        });
        console.log('review:', JSON.stringify(review, null, 2));
        await page.screenshot({ path: join(outputDir, 'desktop-4-review.png'), fullPage: true });
      } catch (err) {
        console.log('review audit skipped:', err.message.slice(0, 120));
      }
    }

    await page.close();
  }

  await browser.close();
  console.log('screenshots in ' + outputDir);
}

main().catch((err) => {
  console.error('FAILED:', err.message);
  process.exit(1);
});
