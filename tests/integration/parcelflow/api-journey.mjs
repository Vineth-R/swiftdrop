/** Real HTTP integration against a disposable local gateway; no mocked replies. */
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import test from 'node:test';
import { withSafeErrors } from './safe-errors.mjs';
import { request } from '@playwright/test';

test(
  'real pricing, parcel persistence, idempotency and customer ownership',
  { timeout: 120_000 },
  withSafeErrors(async () => {
    assert.equal(
      process.env.M5_LIVE_DISPOSABLE,
      '1',
      'M5_LIVE_DISPOSABLE=1 must confirm a disposable local stack.',
    );
    const baseURL = process.env.M5_LIVE_BASE_URL;
    assert.ok(baseURL, 'M5_LIVE_BASE_URL is required');
    const target = new URL(baseURL);
    assert.ok(
      target.protocol === 'http:' &&
        ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname),
      'Only a local HTTP test stack is allowed.',
    );
    const evidence = process.env.M5_LIVE_EVIDENCE ?? 'test-output/m5-live';
    await mkdir(evidence, { recursive: true });
    const customer = await request.newContext({ baseURL });
    const outsider = await request.newContext({ baseURL });
    const admin = await request.newContext({ baseURL });
    let restoreRule;
    let ruleID;
    try {
      const password = `M5api${randomUUID()}A!`;
      for (const [index, client] of [customer, outsider].entries()) {
        const email = `m5.api.${randomUUID()}@example.test`;
        assert.equal(
          (
            await client.post('/api/v1/auth/register', {
              data: {
                email,
                password,
                fullName: 'M5 API Customer',
                phone: `+9477123456${index}`,
              },
            })
          ).status(),
          201,
        );
        assert.equal(
          (
            await client.post('/api/v1/auth/login', {
              data: { email, password },
            })
          ).status(),
          200,
        );
      }
      const zonesReply = await customer.get('/api/v1/zones');
      assert.equal(zonesReply.status(), 200);
      const zones = (await zonesReply.json()).data;
      const senderZone = zones.find((zone) => zone.code === 'CMB');
      const recipientZone = zones.find((zone) => zone.code === 'KDY');
      assert.ok(senderZone && recipientZone);
      const quoteInput = {
        originZoneId: senderZone.id,
        destinationZoneId: recipientZone.id,
        weightKg: 2,
        sizeCategory: 'SMALL',
      };
      const quoteReply = await customer.post('/api/v1/pricing/estimate', {
        data: quoteInput,
      });
      assert.equal(quoteReply.status(), 200);
      const quote = (await quoteReply.json()).data;
      const input = {
        sender: {
          name: 'Nimal Perera',
          phone: '+94771234567',
          address: '12 Temple Road',
          city: 'Colombo',
          zoneId: senderZone.id,
        },
        recipient: {
          name: 'Amali Fernando',
          phone: '+94712345678',
          address: '45 Lake Drive',
          city: 'Kandy',
          zoneId: recipientZone.id,
        },
        weightKg: 2,
        sizeCategory: 'SMALL',
        description: 'M5 disposable integration books',
        expectedFee: quote.total,
      };
      const key = randomUUID();
      const create = () =>
        customer.post('/api/v1/parcels', {
          data: input,
          headers: { 'Idempotency-Key': key },
        });
      const replies = await Promise.all(Array.from({ length: 8 }, create));
      assert.ok(
        replies.every((reply) => reply.status() === 201),
        'All concurrent identical requests return 201',
      );
      const records = await Promise.all(
        replies.map(async (reply) => (await reply.json()).data),
      );
      const parcel = records[0];
      assert.ok(records.every((record) => record.id === parcel.id));
      assert.match(parcel.trackingNumber, /^SDK[A-HJ-NP-Z2-9]{8}$/);
      assert.equal(parcel.fee.total, quote.total);
      assert.equal(parcel.status, 'CREATED');
      const detailReply = await customer.get(`/api/v1/parcels/${parcel.id}`);
      assert.equal(detailReply.status(), 200);
      assert.equal(
        (await detailReply.json()).data.sender.address,
        input.sender.address,
      );
      assert.equal(detailReply.headers()['cache-control'], 'no-store');
      assert.ok(detailReply.headers()['x-request-id']);
      const historyReply = await customer.get(
        `/api/v1/parcels/${parcel.id}/history`,
      );
      assert.equal(historyReply.status(), 200);
      const history = (await historyReply.json()).data;
      assert.equal(history.length, 1);
      assert.equal(history[0].status, 'CREATED');
      assert.equal(history[0].previousStatus, null);
      const listReply = await customer.get(
        '/api/v1/parcels?status=CREATED&page=1&pageSize=1&sort=createdAt:desc',
      );
      assert.equal(listReply.status(), 200);
      const list = await listReply.json();
      assert.equal(list.data[0].id, parcel.id);
      assert.equal(list.meta.pagination.totalItems, 1);
      for (const suffix of ['', '/history', '/receipt']) {
        const denied = await outsider.get(
          `/api/v1/parcels/${parcel.id}${suffix}`,
        );
        assert.equal(denied.status(), 404);
        assert.equal((await denied.json()).error.code, 'NOT_FOUND');
      }
      assert.equal((await customer.get('/internal/parcels')).status(), 404);
      assert.equal(
        (await customer.get('/api/v1/internal/parcels')).status(),
        404,
      );
      const anonymous = await request.newContext({ baseURL });
      try {
        assert.equal((await anonymous.get('/api/v1/parcels')).status(), 401);
      } finally {
        await anonymous.dispose();
      }
      const receiptReply = await customer.get(
        `/api/v1/parcels/${parcel.id}/receipt`,
      );
      assert.equal(receiptReply.status(), 200);
      assert.equal(receiptReply.headers()['content-type'], 'application/pdf');
      assert.equal(
        receiptReply.headers()['content-disposition'],
        `attachment; filename="receipt_${parcel.trackingNumber}.pdf"`,
      );
      const receipt = await receiptReply.body();
      assert.equal(receipt.subarray(0, 8).toString(), '%PDF-1.4');
      await writeFile(join(evidence, 'api-receipt.pdf'), receipt);

      let pricingMutation =
        'NOT TESTED: M5_LIVE_ADMIN_EMAIL/PASSWORD not supplied';
      if (
        process.env.M5_LIVE_ADMIN_EMAIL &&
        process.env.M5_LIVE_ADMIN_PASSWORD
      ) {
        assert.equal(
          (
            await admin.post('/api/v1/auth/login', {
              data: {
                email: process.env.M5_LIVE_ADMIN_EMAIL,
                password: process.env.M5_LIVE_ADMIN_PASSWORD,
              },
            })
          ).status(),
          200,
        );
        const rulesReply = await admin.get(
          `/api/v1/admin/pricing/rules?originZoneId=${senderZone.id}&destinationZoneId=${recipientZone.id}&isActive=true&pageSize=100`,
        );
        assert.equal(rulesReply.status(), 200);
        const rule = (await rulesReply.json()).data.find(
          (row) =>
            row.originZoneId === senderZone.id &&
            row.destinationZoneId === recipientZone.id &&
            row.isActive,
        );
        assert.ok(rule);
        ruleID = rule.id;
        restoreRule = { baseFee: rule.baseFee };
        assert.equal(
          (
            await admin.patch(`/api/v1/admin/pricing/rules/${ruleID}`, {
              data: { baseFee: rule.baseFee + 50 },
            })
          ).status(),
          200,
        );
        const newQuote = (
          await (
            await customer.post('/api/v1/pricing/estimate', {
              data: quoteInput,
            })
          ).json()
        ).data;
        assert.equal(newQuote.total, quote.total + 50);
        const mismatch = await customer.post('/api/v1/parcels', {
          data: input,
          headers: { 'Idempotency-Key': randomUUID() },
        });
        assert.equal(mismatch.status(), 409);
        const error = (await mismatch.json()).error;
        assert.equal(error.code, 'PRICE_CHANGED');
        assert.equal(error.currentFee, newQuote.total);
        const replayReply = await create();
        assert.equal(replayReply.status(), 201);
        assert.equal((await replayReply.json()).data.fee.total, quote.total);
        assert.equal(
          (await (await customer.get(`/api/v1/parcels/${parcel.id}`)).json())
            .data.fee.total,
          quote.total,
        );
        const unchangedReceipt = await (
          await customer.get(`/api/v1/parcels/${parcel.id}/receipt`)
        ).body();
        assert.deepEqual(unchangedReceipt, receipt);
        pricingMutation =
          'PASS: real admin update, changed quote, PRICE_CHANGED, replay/detail/receipt original fee';
      }
      const outcome = {
        result: 'PASS',
        parcelId: parcel.id,
        trackingNumber: parcel.trackingNumber,
        originalTotal: quote.total,
        concurrentRequests: 8,
        pricingMutation,
        requestsMocked: 0,
      };
      await writeFile(
        join(evidence, 'api-journey-result.json'),
        JSON.stringify(outcome, null, 2),
      );
      console.log(JSON.stringify(outcome));
    } finally {
      if (restoreRule && ruleID)
        assert.equal(
          (
            await admin.patch(`/api/v1/admin/pricing/rules/${ruleID}`, {
              data: restoreRule,
            })
          ).status(),
          200,
          'Restore original disposable pricing rule',
        );
      await customer.dispose();
      await outsider.dispose();
      await admin.dispose();
    }
  }),
);
