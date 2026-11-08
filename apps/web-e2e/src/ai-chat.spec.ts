import { expect, test, type Page } from '@playwright/test';

import {
  fail,
  mockApi,
  ok,
  SERVER_WORDS,
  type Handler,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

const CHAT = 'POST /ai/chat';
const CONVERSATION_A = 'cv81a2c4e6f8091b2c3d4e5f60718293a4';
const CONVERSATION_B = 'cv81a2c4e6f8091b2c3d4e5f60718293a5';

const launcher = (page: Page) =>
  page.getByRole('button', { name: 'Open the SwiftDrop Delivery Assistant' });
const panel = (page: Page) =>
  page.getByRole('dialog', { name: 'SwiftDrop Delivery Assistant' });
const input = (page: Page) =>
  page.getByLabel('Message the SwiftDrop Delivery Assistant');
const sendButton = (page: Page) =>
  page.getByRole('button', { name: 'Send message' });

const reply = (
  text: string,
  conversationId = CONVERSATION_A,
  toolCalls: Array<{ name: string; status: string }> = [],
) => ok({ conversationId, reply: text, toolCalls });

async function openAssistant(page: Page) {
  await launcher(page).click();
  await expect(panel(page)).toBeVisible();
}

test.describe('SwiftDrop Delivery Assistant widget', () => {
  test('opens from every page and closes back to the launcher', async ({
    page,
  }) => {
    await mockApi(page, {});
    await page.goto('/track');

    await expect(launcher(page)).toBeVisible();
    await openAssistant(page);
    await expect(input(page)).toBeFocused();

    await page.getByRole('button', { name: 'Close the assistant' }).click();
    await expect(panel(page)).toHaveCount(0);
    await expect(launcher(page)).toBeFocused();
  });

  test('shows the identity, welcome copy and every suggested question', async ({
    page,
  }) => {
    await mockApi(page, {});
    await page.goto('/track');
    await openAssistant(page);

    await expect(
      page.getByRole('heading', {
        level: 2,
        name: 'SwiftDrop Delivery Assistant',
      }),
    ).toBeVisible();
    await expect(panel(page)).toContainText(
      "Hi! I'm your SwiftDrop Delivery Assistant.",
    );
    await expect(
      panel(page)
        .getByRole('list', { name: 'Suggested questions' })
        .getByRole('listitem'),
    ).toHaveCount(5);
    for (const question of [
      'What is SwiftDrop?',
      'How does delivery pricing work?',
      'Track my parcel.',
      'Show my active deliveries.',
      'What are your terms and conditions?',
    ]) {
      await expect(
        panel(page).getByRole('button', { name: question }),
      ).toBeVisible();
    }
  });

  test('a suggested question sends a real message and renders the reply', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {
      [CHAT]: reply('SwiftDrop is a parcel delivery platform.'),
    });
    await page.goto('/track');
    await openAssistant(page);

    await panel(page)
      .getByRole('button', { name: 'What is SwiftDrop?' })
      .click();

    await expect(
      panel(page).getByText('SwiftDrop is a parcel delivery platform.'),
    ).toBeVisible();
    expect(calls.filter((call) => call.key === CHAT)).toEqual([
      expect.objectContaining({
        body: { message: 'What is SwiftDrop?', conversationId: null },
      }),
    ]);
  });

  test('empty and whitespace-only messages never leave the panel', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {});
    await page.goto('/track');
    await openAssistant(page);

    await expect(sendButton(page)).toBeDisabled();
    await input(page).fill('   ');
    await expect(sendButton(page)).toBeDisabled();
    await input(page).press('Enter');

    await expect(sendButton(page)).toBeDisabled();
    expect(calls.filter((call) => call.key === CHAT)).toEqual([]);
  });

  test('shows a typing state and blocks a second send while one is in flight', async ({
    page,
  }) => {
    await mockApi(page, {
      [CHAT]: { ...reply('Here is the answer.'), delayMs: 800 },
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText('The assistant is typing');
    await expect(sendButton(page)).toBeDisabled();
    await expect(panel(page).getByText('Here is the answer.')).toBeVisible();
    await expect(panel(page)).not.toContainText('The assistant is typing');
  });

  test('keeps the conversation ID across a follow-up', async ({ page }) => {
    const { calls } = await mockApi(page, {
      [CHAT]: reply('SwiftDrop is a parcel delivery platform.'),
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');
    await expect(
      panel(page).getByText('SwiftDrop is a parcel delivery platform.'),
    ).toBeVisible();

    await input(page).fill('How long does delivery take?');
    await input(page).press('Enter');
    await expect(
      panel(page).getByText('SwiftDrop is a parcel delivery platform.'),
    ).toHaveCount(2);

    const chatCalls = calls.filter((call) => call.key === CHAT);
    expect(chatCalls[0].body).toEqual({
      message: 'What is SwiftDrop?',
      conversationId: null,
    });
    expect(chatCalls[1].body).toEqual({
      message: 'How long does delivery take?',
      conversationId: CONVERSATION_A,
    });
  });

  test('a 422 explains the message is invalid without server words', async ({
    page,
  }) => {
    await mockApi(page, {
      [CHAT]: fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'message', issue: 'too long' }],
      }),
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('A message');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText('That message could not be sent');
    await expect(input(page)).toHaveValue('A message');
    await expect(
      panel(page).getByRole('button', { name: 'Try again' }),
    ).toHaveCount(0);
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a 429 states the wait and keeps Send closed', async ({ page }) => {
    await mockApi(page, {
      [CHAT]: fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '20' },
      }),
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('A message');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText('Too many messages');
    await expect(panel(page)).toContainText('20 seconds');
    await expect(panel(page)).toContainText(/Try again in \d+ seconds/);
    await expect(sendButton(page)).toBeDisabled();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a 503 shows the safe provider outage sentence', async ({ page }) => {
    await mockApi(page, { [CHAT]: fail(503, 'AI_UNAVAILABLE') });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText(
      'The assistant is temporarily unavailable. Please try again later.',
    );
    await expect(
      panel(page).getByRole('button', { name: 'Try again' }),
    ).toBeVisible();
  });

  test('a lost connection can be retried, and the retry reuses the message', async ({
    page,
  }) => {
    let isDown = true;
    const handler: Handler = () =>
      isDown
        ? fail(500, 'INTERNAL_ERROR')
        : reply('SwiftDrop is a parcel delivery platform.');
    const { calls } = await mockApi(page, { [CHAT]: handler });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');
    await expect(panel(page)).toContainText('Something broke on our side');

    isDown = false;
    await panel(page).getByRole('button', { name: 'Try again' }).click();
    await expect(
      panel(page).getByText('SwiftDrop is a parcel delivery platform.'),
    ).toBeVisible();

    const chatCalls = calls.filter((call) => call.key === CHAT);
    expect(chatCalls).toHaveLength(2);
    expect(chatCalls[1].body).toEqual({
      message: 'What is SwiftDrop?',
      conversationId: null,
    });
    await expect(
      panel(page).getByRole('button', { name: 'Try again' }),
    ).toHaveCount(0);
  });

  test('the network failing says so without a server message', async ({
    page,
  }) => {
    await page.route('**/api/v1/ai/chat', (route) => route.abort());
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText('We could not reach SwiftDrop');
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('an anonymous request for private history is refused with a sign-in note', async ({
    page,
  }) => {
    await mockApi(page, {
      [CHAT]: reply(
        'Please sign in to see your own parcel history.',
        CONVERSATION_A,
        [{ name: 'get_my_parcel_history', status: 'DENIED' }],
      ),
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('Show my parcel history.');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText(
      'Please sign in to see your own parcel history.',
    );
    await expect(panel(page)).toContainText(
      'Sign in to use your parcel history.',
    );
  });

  test('a signed-in customer can ask for their active deliveries', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      [CHAT]: reply(
        'You have one active delivery, SDK7M2Q9XP4, in transit to Kandy.',
        CONVERSATION_A,
        [{ name: 'get_my_active_deliveries', status: 'OK' }],
      ),
    });
    await page.goto('/dashboard');
    await openAssistant(page);

    await input(page).fill('Show my active deliveries.');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText(
      'You have one active delivery, SDK7M2Q9XP4, in transit to Kandy.',
    );
    expect(calls.filter((call) => call.key === CHAT)).toHaveLength(1);
  });

  test('an expired conversation starts a new one and retries once', async ({
    page,
  }) => {
    let call = 0;
    const { calls } = await mockApi(page, {
      [CHAT]: () => {
        call += 1;
        if (call === 1) return reply('First answer.');
        if (call === 2) return fail(404, 'CONVERSATION_NOT_FOUND');
        return reply('Second answer.', CONVERSATION_B);
      },
    });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('First question');
    await input(page).press('Enter');
    await expect(panel(page).getByText('First answer.')).toBeVisible();

    await input(page).fill('Second question');
    await input(page).press('Enter');
    await expect(panel(page).getByText('Second answer.')).toBeVisible();

    const chatCalls = calls.filter((call) => call.key === CHAT);
    expect(chatCalls).toHaveLength(3);
    expect(chatCalls[1].body).toEqual({
      message: 'Second question',
      conversationId: CONVERSATION_A,
    });
    expect(chatCalls[2].body).toEqual({
      message: 'Second question',
      conversationId: null,
    });
  });

  test('an unexpected response shape is a fault, not a blank bubble', async ({
    page,
  }) => {
    await mockApi(page, { [CHAT]: ok({ nonsense: true }) });
    await page.goto('/track');
    await openAssistant(page);

    await input(page).fill('What is SwiftDrop?');
    await input(page).press('Enter');

    await expect(panel(page)).toContainText('Something broke on our side');
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('the panel fits a phone without sideways scrolling', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await mockApi(page, {});
    await page.goto('/track');
    await openAssistant(page);

    const box = await panel(page).boundingBox();
    expect(box?.x).toBeGreaterThanOrEqual(0);
    expect(box?.y).toBeGreaterThanOrEqual(0);
    expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(375);
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
  });

  test('the launcher clears the courier mobile bottom tabs', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await signInAs(page, 'COURIER');
    await page.goto('/dashboard');

    const profile = page.getByRole('link', { name: 'Profile' });
    await expect(profile).toBeVisible();
    const launcherBox = await launcher(page).boundingBox();
    const profileBox = await profile.boundingBox();
    expect(launcherBox).not.toBeNull();
    expect(profileBox).not.toBeNull();
    const overlap =
      launcherBox &&
      profileBox &&
      launcherBox.x < profileBox.x + profileBox.width &&
      profileBox.x < launcherBox.x + launcherBox.width &&
      launcherBox.y < profileBox.y + profileBox.height &&
      profileBox.y < launcherBox.y + launcherBox.height;
    expect(overlap).toBe(false);
  });

  test('the panel fits a landscape phone and keeps Close reachable', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 812, height: 375 });
    await mockApi(page, {});
    await page.goto('/track');
    await openAssistant(page);

    const box = await panel(page).boundingBox();
    expect(box?.y).toBeGreaterThanOrEqual(0);
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(375);
    await expect(
      page.getByRole('button', { name: 'Close the assistant' }),
    ).toBeVisible();
  });

  test('works from the keyboard and returns focus on Escape', async ({
    page,
  }) => {
    await mockApi(page, {
      [CHAT]: reply('SwiftDrop is a parcel delivery platform.'),
    });
    await page.goto('/track');
    await launcher(page).focus();
    await page.keyboard.press('Enter');

    await expect(panel(page)).toBeVisible();
    await expect(input(page)).toBeFocused();

    await input(page).fill('What is SwiftDrop?');
    await page.keyboard.press('Enter');
    await expect(
      panel(page).getByText('SwiftDrop is a parcel delivery platform.'),
    ).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(panel(page)).toHaveCount(0);
    await expect(launcher(page)).toBeFocused();
  });
});
