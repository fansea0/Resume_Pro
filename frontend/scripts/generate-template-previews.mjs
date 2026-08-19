#!/usr/bin/env node
/**
 * Renders every registered template with `createSampleResumeDocument()` and saves a
 * PNG cover to `frontend/public/template-previews/<id>.png`. Uses the system
 * Chrome (no bundled browser) and rasterizes the actual `.resume-template` DOM
 * node — not a hand-drawn SVG — so the cover matches what `exportResumePng`
 * produces for a real export.
 *
 * Usage:
 *   node scripts/generate-template-previews.mjs              # all templates
 *   node scripts/generate-template-previews.mjs --only=scholarly-serif
 *
 * Requirements:
 *   - `npm run build` (dist/ must exist)
 *   - puppeteer-core in devDependencies
 *   - Google Chrome installed at /Applications/Google Chrome.app (macOS default)
 */

import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const frontendDir = resolve(__dirname, '..');
const projectRoot = resolve(frontendDir, '..');
const distDir = join(frontendDir, 'dist');
const outputDir = join(frontendDir, 'public', 'template-previews');

const PREVIEW_PORT = 4180;
const PREVIEW_HOST = '127.0.0.1';
const PREVIEW_URL = `http://${PREVIEW_HOST}:${PREVIEW_PORT}/`;
const CHROME_PATHS = [
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium-browser',
  '/usr/bin/chromium',
];

function findChrome() {
  for (const path of CHROME_PATHS) {
    if (existsSync(path)) return path;
  }
  throw new Error(
    'No Chrome binary found. Install Google Chrome or set CHROME_PATH environment variable.'
  );
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForServer(url, timeoutMs = 30000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url);
      if (res.ok || res.status === 304) return;
    } catch {
      /* not ready yet */
    }
    await sleep(250);
  }
  throw new Error(`Server at ${url} did not become ready within ${timeoutMs}ms.`);
}

function startPreviewServer() {
  // `vite preview` requires the dist/ build to exist.
  if (!existsSync(distDir)) {
    throw new Error(
      `Build directory not found: ${distDir}. Run \`npm run build\` before generating previews.`
    );
  }
  const child = spawn(
    'npx',
    ['vite', 'preview', '--port', String(PREVIEW_PORT), '--strictPort', '--host', PREVIEW_HOST],
    { cwd: frontendDir, stdio: ['ignore', 'pipe', 'pipe'] }
  );
  child.stdout.on('data', (chunk) => process.stdout.write(`[vite] ${chunk}`));
  child.stderr.on('data', (chunk) => process.stderr.write(`[vite] ${chunk}`));
  return child;
}

function parseArgs(argv) {
  const args = { only: null };
  for (const arg of argv.slice(2)) {
    if (arg.startsWith('--only=')) {
      args.only = arg.slice('--only='.length).trim();
    } else if (arg === '--help' || arg === '-h') {
      args.help = true;
    }
  }
  return args;
}

function printHelp() {
  console.log(`generate-template-previews.mjs

Generate the static PNG cover for every template (or one with --only=<id>).

Options:
  --only=<id>     Generate cover for a single template id
  --help, -h      Show this help
`);
}

async function captureTemplate(browser, templateId) {
  const page = await browser.newPage();
  try {
    await page.setViewport({ width: 900, height: 1280, deviceScaleFactor: 2 });
    const target = `${PREVIEW_URL}?preview=${encodeURIComponent(templateId)}`;
    await page.goto(target, { waitUntil: 'networkidle0' });
    // Wait for the actual template root to mount, then give fonts a tick.
    await page.waitForSelector('.resume-template', { timeout: 15000 });
    await page.evaluate(() => document.fonts?.ready);
    // Give layout a tick to settle after fonts load. `requestAnimationFrame` is
    // not available in Node, so schedule it inside the page.
    await page.evaluate(
      () =>
        new Promise((resolve) => {
          window.requestAnimationFrame(() => resolve());
        })
    );
    const node = await page.$('.resume-template');
    if (!node) throw new Error(`No .resume-template element rendered for ${templateId}.`);
    const buffer = await node.screenshot({ type: 'png' });
    const outPath = join(outputDir, `${templateId}.png`);
    await mkdir(outputDir, { recursive: true });
    await writeFile(outPath, buffer);
    console.log(`  ✓ ${templateId} → ${outPath}`);
  } finally {
    await page.close();
  }
}

async function main() {
  const args = parseArgs(process.argv);
  if (args.help) {
    printHelp();
    return;
  }
  if (!existsSync(distDir)) {
    throw new Error(`Build directory not found: ${distDir}. Run \`npm run build\` first.`);
  }
  await mkdir(outputDir, { recursive: true });

  const server = startPreviewServer();
  const cleanup = () => {
    if (!server.killed) server.kill('SIGTERM');
  };
  process.on('exit', cleanup);
  process.on('SIGINT', () => {
    cleanup();
    process.exit(130);
  });
  process.on('SIGTERM', () => {
    cleanup();
    process.exit(143);
  });

  try {
    await waitForServer(PREVIEW_URL);
    console.log(`Preview server ready at ${PREVIEW_URL}`);

    const browser = await puppeteer.launch({
      executablePath: process.env.CHROME_PATH ?? findChrome(),
      headless: 'new',
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });
    try {
      // Discover the registry by evaluating the running app.
      const page = await browser.newPage();
      await page.goto(PREVIEW_URL, { waitUntil: 'networkidle0' });
      const ids = await page.evaluate(() => window.__TEMPLATE_IDS__ ?? null);
      await page.close();
      if (!Array.isArray(ids) || ids.length === 0) {
        throw new Error(
          'Could not read template ids from the running app. ' +
            'Make sure App exposes `window.__TEMPLATE_IDS__`.'
        );
      }
      const targets = args.only ? ids.filter((id) => id === args.only) : ids;
      if (args.only && targets.length === 0) {
        throw new Error(`Template "${args.only}" not found. Known ids: ${ids.join(', ')}`);
      }
      console.log(`Generating covers for ${targets.length} template(s)…`);
      for (const id of targets) {
        await captureTemplate(browser, id);
      }
    } finally {
      await browser.close();
    }
  } finally {
    cleanup();
  }
}

main().catch((err) => {
  console.error(err.stack || err.message);
  process.exit(1);
});
