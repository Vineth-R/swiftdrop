#!/usr/bin/env node
// Renders the editable merchant API guide to a print-ready PDF with the
// Playwright Chromium already used by the E2E suite. No Pandoc or LaTeX.
import { copyFile, mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

import { chromium } from '@playwright/test';

const root = process.cwd();
const source = resolve(root, 'docs/merchant-api-guide.html');
const pdf = resolve(root, 'docs/merchant-api-guide.pdf');
const publicCopy = resolve(root, 'apps/web/public/merchant-api-guide.pdf');

await mkdir(resolve(root, 'docs'), { recursive: true });
const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  await page.goto(pathToFileURL(source).href, { waitUntil: 'load' });
  await page.pdf({
    path: pdf,
    format: 'A4',
    printBackground: true,
    displayHeaderFooter: true,
    headerTemplate: '<div></div>',
    footerTemplate: `<div style="width:100%;padding:0 18mm;display:flex;justify-content:space-between;color:#64748b;font:8px -apple-system,Segoe UI,Arial,sans-serif;">
      <span>SwiftDrop Merchant API Guide</span>
      <span>Page <span class="pageNumber"></span> of <span class="totalPages"></span></span>
    </div>`,
    margin: { top: '18mm', bottom: '20mm', left: '18mm', right: '18mm' },
  });
} finally {
  await browser.close();
}
await copyFile(pdf, publicCopy);
console.log(
  'wrote docs/merchant-api-guide.pdf and apps/web/public/merchant-api-guide.pdf',
);
