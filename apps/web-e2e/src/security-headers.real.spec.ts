import { expect, test } from '@playwright/test';

/**
 * Real-stack browser security regression: the gateway must send the Phase 2
 * headers, the app must load without CSP violations, same-origin mutations must
 * work, cross-site mutations must be rejected, and the WebSocket must connect.
 */
test.describe('gateway security regression', () => {
  test('security headers and CSP are present and the page loads cleanly', async ({
    page,
  }) => {
    const violations: string[] = [];
    page.on('console', (message) => {
      if (
        message.type() === 'error' ||
        /Content Security Policy/i.test(message.text())
      ) {
        violations.push(message.text());
      }
    });
    page.on('pageerror', (error) => violations.push(error.message));

    const response = await page.goto('/');
    expect(response?.ok()).toBeTruthy();
    const headers = response?.headers() ?? {};
    expect(headers['content-security-policy']).toContain("default-src 'self'");
    expect(headers['content-security-policy']).toContain("object-src 'none'");
    expect(headers['x-content-type-options']).toBe('nosniff');
    expect(headers['x-frame-options']).toBe('SAMEORIGIN');
    expect(headers['permissions-policy']).toContain('camera=()');
    expect(headers['cross-origin-opener-policy']).toBe('same-origin');
    expect(violations).toEqual([]);
  });

  test('same-origin mutation reaches the API while cross-site is rejected', async ({
    page,
    request,
  }) => {
    await page.goto('/');
    const baseURL = new URL(page.url()).origin;
    const body = {
      email: 'phase3-security@example.test',
      password: 'not-the-password',
    };

    const crossSite = await request.post('/api/v1/auth/login', {
      headers: {
        Origin: 'https://evil.example',
        'Sec-Fetch-Site': 'cross-site',
      },
      data: body,
    });
    expect(crossSite.status()).toBe(403);
    expect(await crossSite.text()).toContain('CSRF_ORIGIN_MISMATCH');

    const sameOrigin = await request.post('/api/v1/auth/login', {
      headers: {
        Origin: baseURL,
        'Sec-Fetch-Site': 'same-origin',
      },
      data: body,
    });
    expect(sameOrigin.status()).toBe(401);
  });

  test('the anonymous realtime WebSocket opens from the page origin', async ({
    page,
  }) => {
    await page.goto('/track');
    const wsBase = new URL(page.url()).origin.replace(/^http/, 'ws');
    const result = await page.evaluate(
      (base) =>
        new Promise<string>((resolve) => {
          const socket = new WebSocket(`${base}/ws`);
          const timer = setTimeout(() => {
            socket.close();
            resolve('timeout');
          }, 8000);
          socket.onopen = () => {
            clearTimeout(timer);
            socket.close();
            resolve('open');
          };
          socket.onerror = () => {
            clearTimeout(timer);
            resolve('error');
          };
        }),
      wsBase,
    );
    expect(result).toBe('open');
  });
});
