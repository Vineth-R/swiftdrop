import assert from 'node:assert/strict';
import { existsSync, readFileSync, statSync } from 'node:fs';
import test from 'node:test';

const htmlPath = 'docs/merchant-api-guide.html';
const pdfPath = 'docs/merchant-api-guide.pdf';
const publicPdfPath = 'apps/web/public/merchant-api-guide.pdf';

test('the merchant API guide source documents the implemented integration surface', () => {
  const html = readFileSync(htmlPath, 'utf8');
  for (const required of [
    'X-API-Key',
    'GET /api/v1/zones',
    'POST /api/v1/merchant/orders',
    'GET /api/v1/merchant/orders',
    'GET /api/v1/merchant/orders/{trackingNumber}/receipt',
    'parcel.status_changed',
    'X-SwiftDrop-Signature',
    'Idempotency-Key',
    'orderId',
  ]) {
    assert.ok(
      html.includes(required),
      `${htmlPath} does not document ${required}`,
    );
  }
});

test('the generated merchant API guide PDF is a readable, committed deliverable', () => {
  assert.ok(existsSync(pdfPath), `${pdfPath} is missing`);
  assert.ok(existsSync(publicPdfPath), `${publicPdfPath} is missing`);
  const pdf = readFileSync(pdfPath);
  assert.ok(pdf.length > 10_000, `${pdfPath} is unexpectedly small`);
  assert.ok(
    pdf.subarray(0, 5).toString() === '%PDF-',
    `${pdfPath} is not a PDF`,
  );
  assert.ok(
    pdf.toString('latin1').includes('%%EOF'),
    `${pdfPath} is truncated`,
  );
  assert.equal(
    statSync(pdfPath).size,
    statSync(publicPdfPath).size,
    'the public PDF is not the generated guide',
  );
});
