/**
 * Real browser + gateway + Identity + Pricing + Parcel + PostgreSQL smoke test.
 * No request interception or mocked responses. Run only against a disposable
 * local stack; it creates two real customer accounts and a parcel.
 *
 * M5_LIVE_DISPOSABLE=1 M5_LIVE_BASE_URL=http://localhost:3000 \
 *   node --test tests/integration/parcelflow/customer-journey.mjs
 */
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import test from 'node:test';
import { withSafeErrors } from './safe-errors.mjs';
import { chromium, expect as playwrightExpect } from '@playwright/test';

const expect = playwrightExpect.configure({ timeout: 30_000 });

test(
  'real customer creates, lists, tracks and downloads an owned parcel',
  { timeout: 240_000 },
  withSafeErrors(async () => {
    assert.equal(
      process.env.M5_LIVE_DISPOSABLE,
      '1',
      'Explicitly confirm a disposable local stack with M5_LIVE_DISPOSABLE=1.',
    );
    const baseURL = process.env.M5_LIVE_BASE_URL;
    assert.ok(baseURL, 'M5_LIVE_BASE_URL is required.');
    const target = new URL(baseURL);
    assert.ok(
      target.protocol === 'http:' &&
        ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname),
      'Only a local HTTP test stack is allowed.',
    );
    const evidence = process.env.M5_LIVE_EVIDENCE ?? 'test-output/m5-live';
    await mkdir(evidence, { recursive: true });
    const browser = await chromium.launch({
      channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
      headless: true,
      args: process.env.M5_BROWSER_DISABLE_GPU === '1' ? ['--disable-gpu'] : [],
    });
    const context = await browser.newContext({
      baseURL,
      viewport: { width: 1280, height: 900 },
      acceptDownloads: true,
    });
    const other = await browser.newContext({ baseURL });
    const page = await context.newPage();
    page.setDefaultTimeout(30_000);
    const password = `M5live${randomUUID()}A!`;
    const email = `m5.live.${randomUUID()}@example.test`;
    try {
      await page.goto('/register?next=/new');
      await page
        .getByLabel('Full name', { exact: false })
        .fill('M5 Live Customer');
      await page.getByLabel('Email', { exact: false }).fill(email);
      await page.getByLabel('Phone', { exact: false }).fill('771234567');
      await page.getByLabel(/^Password/).fill(password);
      await page
        .getByLabel('Confirm password', { exact: false })
        .fill(password);
      await page
        .getByRole('button', { name: 'Create account', exact: true })
        .click();
      await expect(
        page.getByRole('heading', { name: 'New delivery', exact: true }),
      ).toBeVisible();

      const zonesResponse = await context.request.get('/api/v1/zones');
      assert.equal(zonesResponse.status(), 200);
      const zones = (await zonesResponse.json()).data;
      const origin = zones.find((zone) => zone.code === 'CMB');
      const destination = zones.find((zone) => zone.code === 'KDY');
      assert.ok(
        origin && destination,
        'The documented CMB/KDY reference zones must exist.',
      );
      await page
        .getByLabel('Pickup zone', { exact: false })
        .selectOption(origin.id);
      await page
        .getByLabel('Destination zone', { exact: false })
        .selectOption(destination.id);
      await page.getByLabel('Weight', { exact: false }).fill('2');
      const estimateResponse = await context.request.post(
        '/api/v1/pricing/estimate',
        {
          data: {
            originZoneId: origin.id,
            destinationZoneId: destination.id,
            weightKg: 2,
            sizeCategory: 'SMALL',
          },
        },
      );
      assert.equal(estimateResponse.status(), 200);
      const fee = (await estimateResponse.json()).data;
      await expect(
        page.getByText(`LKR ${fee.total.toFixed(2)}`, { exact: true }),
      ).toBeVisible();
      await page.getByRole('button', { name: 'Continue', exact: true }).click();
      for (const [label, value] of Object.entries({
        'Sender name': 'Nimal Perera',
        'Sender phone': '+94771234567',
        'Sender address': '12 Temple Road',
        'Sender city': 'Colombo',
        'Recipient name': 'Amali Fernando',
        'Recipient phone': '+94712345678',
        'Recipient address': '45 Lake Drive',
        'Recipient city': 'Kandy',
      }))
        await page.getByLabel(label, { exact: false }).fill(value);
      await page
        .getByLabel('Description (optional)', { exact: false })
        .fill('M5 real integration books');
      await page.getByRole('button', { name: 'Continue', exact: true }).click();
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({
        path: join(evidence, 'review-1280.png'),
        fullPage: true,
      });
      const createdResponse = page.waitForResponse(
        (response) =>
          response.url().endsWith('/api/v1/parcels') &&
          response.request().method() === 'POST',
      );
      await page
        .getByRole('button', { name: 'Create delivery', exact: true })
        .click();
      const response = await createdResponse;
      assert.equal(response.status(), 201);
      const created = (await response.json()).data;
      assert.match(created.trackingNumber, /^SDK[A-HJ-NP-Z2-9]{8}$/);
      assert.deepEqual(created.fee, {
        currency: fee.currency,
        baseFee: fee.baseFee,
        weightCharge: fee.weightCharge,
        sizeSurcharge: fee.sizeSurcharge,
        total: fee.total,
      });
      await expect(
        page.getByRole('heading', { name: 'Delivery created', exact: true }),
      ).toBeVisible();
      await page
        .getByRole('link', { name: 'My parcels', exact: true })
        .last()
        .click();
      await expect(
        page.getByRole('heading', { name: 'My parcels', exact: true }),
      ).toBeVisible();
      await page
        .getByRole('link', {
          name: `View parcel ${created.trackingNumber}`,
          exact: true,
        })
        .click();
      await expect(
        page.getByRole('heading', { name: 'Parcel details', exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole('heading', { name: 'Delivery timeline', exact: true }),
      ).toBeVisible();
      await expect(
        page.getByText('Nimal Perera', { exact: true }),
      ).toBeVisible();
      const downloaded = page.waitForEvent('download');
      await page
        .getByRole('button', { name: 'Download PDF receipt', exact: true })
        .click();
      const download = await downloaded;
      assert.equal(
        download.suggestedFilename(),
        `receipt_${created.trackingNumber}.pdf`,
      );
      const receiptPath = join(evidence, download.suggestedFilename());
      await download.saveAs(receiptPath);
      const receipt = await readFile(receiptPath);
      assert.equal(receipt.subarray(0, 8).toString(), '%PDF-1.4');
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({
        path: join(evidence, 'detail-1280.png'),
        fullPage: true,
      });
      await page.setViewportSize({ width: 360, height: 900 });
      await page.evaluate(() => window.scrollTo(0, 0));
      assert.ok(
        await page.evaluate(
          'document.documentElement.scrollWidth <= window.innerWidth',
        ),
      );
      await page.screenshot({
        path: join(evidence, 'detail-360.png'),
        fullPage: true,
      });

      const otherEmail = `m5.other.${randomUUID()}@example.test`;
      assert.equal(
        (
          await other.request.post('/api/v1/auth/register', {
            data: {
              email: otherEmail,
              password,
              fullName: 'Other Live Customer',
              phone: '+94771234568',
            },
          })
        ).status(),
        201,
      );
      assert.equal(
        (
          await other.request.post('/api/v1/auth/login', {
            data: { email: otherEmail, password },
          })
        ).status(),
        200,
      );
      for (const suffix of ['', '/history', '/receipt']) {
        const denied = await other.request.get(
          `/api/v1/parcels/${created.id}${suffix}`,
        );
        assert.equal(denied.status(), 404);
        assert.equal((await denied.json()).error.code, 'NOT_FOUND');
      }
      // This test may use Next's development origin. The API journey separately
      // checks the raw /internal path on the actual public gateway.
      assert.equal(
        (
          await other.request.get(
            `/api/v1/internal/parcels/${created.id}/summary`,
          )
        ).status(),
        404,
      );
      const publicTracking = await other.request.get(
        `/api/v1/tracking/${created.trackingNumber}`,
      );
      assert.equal(publicTracking.status(), 200);
      const publicData = (await publicTracking.json()).data;
      assert.equal(publicData.trackingNumber, created.trackingNumber);
      for (const privateField of [
        'sender',
        'recipient',
        'fee',
        'recipientName',
      ])
        assert.ok(!(privateField in publicData));
      console.log(
        JSON.stringify({
          result: 'PASS',
          parcelId: created.id,
          trackingNumber: created.trackingNumber,
          storedTotal: fee.total,
          receiptPath,
          requestsMocked: 0,
        }),
      );
    } finally {
      await context.close();
      await other.close();
      await browser.close();
    }
  }),
);
