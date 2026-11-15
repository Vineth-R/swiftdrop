import { expect, test, type Page } from '@playwright/test';

import { fail, ok, type Reply } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const base = {
  ownerUserId: '3f1c7a52-0000-4000-8000-0000000000b1',
  registrationNumber: 'PV 123456',
  businessAddress: '88 Galle Road, Colombo 03',
  contactEmail: 'support@shopzone.lk',
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  createdAt: '2026-11-04T08:00:00Z',
};
const approved = {
  ...base,
  id: '3f1c7a52-0000-4000-8000-0000000000a1',
  businessName: 'ShopZone Lanka',
  status: 'APPROVED' as const,
};
const suspendedStuck = {
  ...base,
  id: '3f1c7a52-0000-4000-8000-0000000000a2',
  businessName: 'Stuck Suspended Ltd',
  status: 'SUSPENDED' as const,
  suspensionReason: 'Repeated delivery complaints.',
  identitySync: 'PENDING' as const,
};
const rejectedStuck = {
  ...base,
  id: '3f1c7a52-0000-4000-8000-0000000000a3',
  businessName: 'Stuck Rejected Ltd',
  status: 'REJECTED' as const,
  identitySync: 'PENDING' as const,
};

function list(items: unknown[], totalItems = items.length): Reply {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        requestId: 'req-1',
        pagination: {
          page: 1,
          pageSize: 20,
          totalItems,
          totalPages: Math.max(1, Math.ceil(totalItems / 20)),
        },
      },
    },
  };
}

const row = (page: Page, name: string) =>
  page.getByRole('row').filter({ hasText: name });

test.describe('admin merchants: suspension reason', () => {
  test('a suspension needs a reason, and it is asked for before anything is sent', async ({
    page,
  }) => {
    let current: Record<string, unknown> = approved;
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([approved]),
      [`GET /admin/merchants/${approved.id}`]: () => ok(current),
      [`POST /admin/merchants/${approved.id}/suspend`]: () => {
        current = { ...approved, status: 'SUSPENDED' as const };
        return ok({
          id: approved.id,
          status: 'SUSPENDED',
          identitySync: 'APPLIED',
          alreadyDone: false,
        });
      },
    });
    await page.goto('/admin/merchants');
    await page.getByRole('button', { name: 'Review ShopZone Lanka' }).click();
    const dialog = page.getByRole('dialog', { name: 'ShopZone Lanka' });
    await dialog.getByRole('button', { name: 'Suspend' }).click();

    // Required, not "optional".
    const reason = page.getByLabel('Reason', { exact: false });
    await expect(page.getByLabel('Reason (optional)')).toHaveCount(0);
    await page.getByRole('button', { name: 'Suspend' }).last().click();

    await expect(
      page.getByText('Enter a reason. The merchant sees it in the email.', {
        exact: true,
      }),
    ).toBeVisible();
    expect(calls.filter((call) => call.key.includes('/suspend'))).toHaveLength(
      0,
    );

    await reason.fill('Repeated delivery complaints.');
    await expect(
      page.getByText('Enter a reason. The merchant sees it in the email.', {
        exact: true,
      }),
    ).toHaveCount(0);
    await page.getByRole('button', { name: 'Suspend' }).last().click();

    await expect(page.getByText('ShopZone Lanka is suspended.')).toBeVisible();
    const sent = calls.find((call) => call.key.endsWith('/suspend'));
    expect(sent?.body).toEqual({ reason: 'Repeated delivery complaints.' });
  });

  test('a rejection requires a reason, and cancelling forgets what was typed', async ({
    page,
  }) => {
    const pending = { ...approved, status: 'PENDING' as const };
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([pending]),
      [`GET /admin/merchants/${pending.id}`]: ok(pending),
    });
    await page.goto('/admin/merchants');
    await page.getByRole('button', { name: 'Review ShopZone Lanka' }).click();
    const dialog = page.getByRole('dialog', { name: 'ShopZone Lanka' });
    await dialog.getByRole('button', { name: 'Reject' }).click();

    await page
      .getByRole('button', { name: 'Reject', exact: true })
      .last()
      .click();
    await expect(
      page.getByText('Enter a reason. The merchant sees it in the email.', {
        exact: true,
      }),
    ).toBeVisible();

    await page.getByLabel('Reason').fill('Not a registered company.');
    await page.getByRole('button', { name: 'Cancel' }).click();
    await page.getByRole('button', { name: 'Reject' }).click();

    await expect(page.getByLabel('Reason')).toHaveValue('');
  });
});

test.describe('admin merchants: retrying a sync that did not reach Identity', () => {
  test('only a merchant with a pending sync offers a retry, and the row says why', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([approved, suspendedStuck]),
    });
    await page.goto('/admin/merchants');

    await expect(
      row(page, 'Stuck Suspended Ltd').getByText(
        'Sign-out pending at Identity',
      ),
    ).toBeVisible();
    await expect(
      row(page, 'Stuck Suspended Ltd').getByRole('button', {
        name: 'Retry sync',
      }),
    ).toBeVisible();
    await expect(
      row(page, 'ShopZone Lanka').getByRole('button', { name: 'Retry sync' }),
    ).toHaveCount(0);
  });

  test('retrying a suspension sends the stored reason and clears the row once it applies', async ({
    page,
  }) => {
    let listCalls = 0;
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': () => {
        listCalls += 1;
        return listCalls === 1
          ? list([suspendedStuck])
          : list([{ ...suspendedStuck, identitySync: undefined }]);
      },
      [`POST /admin/merchants/${suspendedStuck.id}/suspend`]: ok({
        id: suspendedStuck.id,
        status: 'SUSPENDED',
        identitySync: 'APPLIED',
        alreadyDone: false,
      }),
    });
    await page.goto('/admin/merchants');

    await page.getByRole('button', { name: 'Retry sync' }).click();

    await expect(
      page.getByText(
        'Identity applied the decision. The merchant is signed out everywhere.',
      ),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retry sync' })).toHaveCount(
      0,
    );
    await expect(page.getByText('Sign-out pending at Identity')).toHaveCount(0);
    const sent = calls.find((call) => call.key.endsWith('/suspend'));
    expect(sent?.body).toEqual({ reason: 'Repeated delivery complaints.' });
  });

  test('retrying a rejection sends no reason, because the stored one is replayed', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([rejectedStuck]),
      [`POST /admin/merchants/${rejectedStuck.id}/reject`]: ok({
        id: rejectedStuck.id,
        status: 'REJECTED',
        identitySync: 'APPLIED',
        alreadyDone: false,
      }),
    });
    await page.goto('/admin/merchants');

    await page.getByRole('button', { name: 'Retry sync' }).click();

    await expect(
      page.getByText('Identity applied the decision.'),
    ).toBeVisible();
    const sent = calls.find((call) => call.key.endsWith('/reject'));
    expect(sent?.body).toEqual({});
  });

  test('a retry that still cannot reach Identity warns instead of claiming success', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([suspendedStuck]),
      [`POST /admin/merchants/${suspendedStuck.id}/suspend`]: ok({
        id: suspendedStuck.id,
        status: 'SUSPENDED',
        identitySync: 'PENDING',
        alreadyDone: false,
      }),
    });
    await page.goto('/admin/merchants');

    await page.getByRole('button', { name: 'Retry sync' }).click();

    await expect(
      page.getByText('Identity could not be reached again.'),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Retry sync' }),
    ).toBeVisible();
  });

  test('a failed retry request says so in plain words', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([suspendedStuck]),
      [`POST /admin/merchants/${suspendedStuck.id}/suspend`]: fail(
        500,
        'INTERNAL_ERROR',
      ),
    });
    await page.goto('/admin/merchants');

    await page.getByRole('button', { name: 'Retry sync' }).click();

    await expect(
      page.getByText('The sync could not be retried. Try again in a moment.'),
    ).toBeVisible();
  });

  test('the decision that leaves a sync pending tells the administrator what to do', async ({
    page,
  }) => {
    let current: Record<string, unknown> = approved;
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([approved]),
      [`GET /admin/merchants/${approved.id}`]: () => ok(current),
      [`POST /admin/merchants/${approved.id}/suspend`]: () => {
        current = {
          ...approved,
          status: 'SUSPENDED' as const,
          identitySync: 'PENDING' as const,
        };
        return ok({
          id: approved.id,
          status: 'SUSPENDED',
          identitySync: 'PENDING',
          alreadyDone: false,
        });
      },
    });
    await page.goto('/admin/merchants');
    await page.getByRole('button', { name: 'Review ShopZone Lanka' }).click();
    const dialog = page.getByRole('dialog', { name: 'ShopZone Lanka' });
    await dialog.getByRole('button', { name: 'Suspend' }).click();
    await page.getByLabel('Reason').fill('Repeated delivery complaints.');
    await page.getByRole('button', { name: 'Suspend' }).last().click();

    await expect(
      page
        .getByRole('dialog', { name: 'ShopZone Lanka' })
        .getByText('Sign-out pending at Identity'),
    ).toBeVisible();
  });
});

test.describe('admin merchants: details, email and re-approval', () => {
  test('clicking a row opens the full merchant details', async ({ page }) => {
    const pending = { ...approved, status: 'PENDING' as const };
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([pending]),
      [`GET /admin/merchants/${pending.id}`]: ok(pending),
    });
    await page.goto('/admin/merchants');

    await row(page, 'ShopZone Lanka').click();

    const dialog = page.getByRole('dialog', { name: 'ShopZone Lanka' });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole('heading', { name: 'Business information' }),
    ).toBeVisible();
    await expect(
      dialog.getByRole('heading', { name: 'Account information' }),
    ).toBeVisible();
    await expect(
      dialog.getByRole('heading', { name: 'Operational information' }),
    ).toBeVisible();
    await expect(dialog.getByText(pending.ownerUserId)).toBeVisible();
    await expect(dialog.getByText(pending.registrationNumber)).toBeVisible();
    await expect(dialog.getByText('PENDING')).toBeVisible();
  });

  test('an administrator can compose and send an email from the details modal', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([approved]),
      [`GET /admin/merchants/${approved.id}`]: ok(approved),
      [`POST /admin/merchants/${approved.id}/email`]: ok({
        message: 'Email sent.',
      }),
    });
    await page.goto('/admin/merchants');
    await row(page, 'ShopZone Lanka').click();
    await page
      .getByRole('dialog', { name: 'ShopZone Lanka' })
      .getByRole('button', { name: 'Compose email' })
      .click();

    const email = page.getByRole('dialog', { name: 'Compose email' });
    await expect(email.getByLabel('To')).toHaveValue(base.contactEmail);
    await email.getByLabel('Subject').fill('Missing registration details');
    await email
      .getByLabel('Message')
      .fill('Please send the missing business registration document.');
    await email.getByRole('button', { name: 'Send email' }).click();

    await expect(email.getByText('Email sent')).toBeVisible();
    const sent = calls.find((call) => call.key.endsWith('/email'));
    expect(sent?.body).toEqual({
      subject: 'Missing registration details',
      message: 'Please send the missing business registration document.',
    });
  });

  test('a rejected merchant shows its reason and can be approved again', async ({
    page,
  }) => {
    const rejected = {
      ...approved,
      status: 'REJECTED' as const,
      rejectionReason: 'Business registration could not be verified.',
    };
    let current: Record<string, unknown> = rejected;
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([rejected]),
      [`GET /admin/merchants/${rejected.id}`]: () => ok(current),
      [`POST /admin/merchants/${rejected.id}/approve`]: () => {
        current = {
          ...approved,
          status: 'APPROVED' as const,
          rejectionReason: undefined,
        };
        return ok({
          id: rejected.id,
          status: 'APPROVED',
          identitySync: 'APPLIED',
          alreadyDone: false,
        });
      },
    });
    await page.goto('/admin/merchants');
    await row(page, 'ShopZone Lanka').click();

    const dialog = page.getByRole('dialog', { name: 'ShopZone Lanka' });
    await expect(dialog.getByText(rejected.rejectionReason)).toBeVisible();
    await dialog.getByRole('button', { name: 'Reactivate' }).click();

    const confirm = page.getByRole('dialog', {
      name: 'Approve ShopZone Lanka?',
    });
    await confirm.getByRole('button', { name: 'Approve' }).click();

    await expect(page.getByText('ShopZone Lanka is approved.')).toBeVisible();
    expect(calls.filter((call) => call.key.endsWith('/approve'))).toHaveLength(
      1,
    );
  });
});

test.describe('admin merchants: paging', () => {
  test('pages through the list with the shared pagination', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list([approved], 45),
    });
    await page.goto('/admin/merchants');

    await expect(page.getByText('Page 1 of 3 · 45 results')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Previous' })).toBeDisabled();

    const next = page.waitForRequest(
      (request) => new URL(request.url()).searchParams.get('page') === '2',
    );
    await page.getByRole('button', { name: 'Next', exact: true }).click();
    await next;
  });
});
