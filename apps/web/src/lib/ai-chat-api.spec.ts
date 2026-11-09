import { describe, expect, it } from 'vitest';

import {
  describeChatFailure,
  isConversationGone,
  toolProgressLabel,
} from './ai-chat-api';
import { ApiError } from './api-errors';

const REQUEST_ID = '7f2c9a14-0b63-4de8-9f10-5c2e8d3a1b77';

describe('describeChatFailure', () => {
  it('states the wait on a rate limit and offers no immediate retry', () => {
    const failure = describeChatFailure(
      new ApiError({ code: 'RATE_LIMITED', status: 429, retryAfter: 30 }),
    );

    expect(failure).toMatchObject({
      title: 'Too many messages',
      canRetry: false,
      waitSeconds: 30,
    });
    expect(failure.message).toContain('30 seconds');
  });

  it('keeps the safe provider outage sentence', () => {
    const failure = describeChatFailure(
      new ApiError({ code: 'AI_UNAVAILABLE', status: 503 }),
    );

    expect(failure.message).toBe(
      'The assistant is temporarily unavailable. Please try again later.',
    );
    expect(failure.canRetry).toBe(true);
  });

  it('does not offer to resend an invalid message', () => {
    const failure = describeChatFailure(
      new ApiError({ code: 'VALIDATION_FAILED', status: 422 }),
    );

    expect(failure.canRetry).toBe(false);
    expect(failure.title).toBe('That message could not be sent');
  });

  it('handles a lost connection', () => {
    const failure = describeChatFailure(
      new ApiError({ code: 'NETWORK', status: 0 }),
    );

    expect(failure).toMatchObject({
      title: 'We could not reach SwiftDrop',
      canRetry: true,
    });
  });

  it('shows a reference only for our own fault', () => {
    const failure = describeChatFailure(
      new ApiError({
        code: 'INTERNAL',
        status: 500,
        requestId: REQUEST_ID,
      }),
    );

    expect(failure.reference).toBe(REQUEST_ID);
    expect(failure.canRetry).toBe(true);
  });

  it('does not show a reference for a non-server 4xx', () => {
    const failure = describeChatFailure(
      new ApiError({
        code: 'NOT_FOUND',
        status: 404,
        requestId: REQUEST_ID,
      }),
    );

    expect(failure.reference).toBeUndefined();
  });

  it('treats an unknown thrown value as a retryable fault', () => {
    expect(describeChatFailure(new Error('boom'))).toMatchObject({
      title: 'Something broke on our side',
      canRetry: true,
    });
  });
});

describe('isConversationGone', () => {
  it('recognises only CONVERSATION_NOT_FOUND', () => {
    expect(
      isConversationGone(
        new ApiError({ code: 'CONVERSATION_NOT_FOUND', status: 404 }),
      ),
    ).toBe(true);
    expect(
      isConversationGone(new ApiError({ code: 'NOT_FOUND', status: 404 })),
    ).toBe(false);
    expect(isConversationGone(new Error('nope'))).toBe(false);
  });
});

describe('toolProgressLabel', () => {
  it('stays quiet when a tool succeeded', () => {
    expect(
      toolProgressLabel({ name: 'get_public_parcel_status', status: 'OK' }),
    ).toBeNull();
  });

  it('explains why a private tool needs a sign-in', () => {
    expect(
      toolProgressLabel({
        name: 'get_my_active_deliveries',
        status: 'DENIED',
      }),
    ).toBe('Sign in to use your active deliveries.');
  });

  it('reports an unavailable dependency without its payload', () => {
    expect(
      toolProgressLabel({
        name: 'calculate_delivery_fee',
        status: 'UNAVAILABLE',
      }),
    ).toBe('Delivery pricing is temporarily unavailable.');
  });

  it('ignores an unknown tool name', () => {
    expect(toolProgressLabel({ name: 'run_sql', status: 'OK' })).toBeNull();
  });
});
