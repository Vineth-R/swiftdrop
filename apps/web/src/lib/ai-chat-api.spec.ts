import { describe, expect, it } from 'vitest';

import {
  describeChatFailure,
  isConversationGone,
  suggestedQuestionsFor,
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

  it('explains a denied tool without exposing internals', () => {
    expect(
      toolProgressLabel({
        name: 'get_active_parcels',
        status: 'DENIED',
      }),
    ).toBe(
      'That information is not available with your current access (parcel list).',
    );
  });

  it('labels every approved tool and its invalid state', () => {
    const expected: Record<string, string> = {
      calculate_delivery_fee: 'delivery pricing',
      get_public_parcel_status: 'parcel status',
      get_parcel_history: 'parcel history',
      get_active_parcels: 'parcel list',
      get_parcel_details: 'parcel details',
      search_parcels: 'parcel search',
      list_unassigned_parcels: 'unassigned parcels',
      get_courier_workload: 'courier workload',
      get_my_courier_deliveries: 'your deliveries',
      get_my_courier_delivery: 'your delivery details',
      get_my_merchant_orders: 'your orders',
      get_my_webhook_deliveries: 'webhook deliveries',
    };
    for (const [name, subject] of Object.entries(expected)) {
      expect(toolProgressLabel({ name, status: 'INVALID' })).toBe(
        `More details are needed for ${subject}.`,
      );
    }
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

describe('suggestedQuestionsFor', () => {
  it('gives guests only public starting points', () => {
    expect(suggestedQuestionsFor(null)).toEqual([
      'Track a parcel.',
      'Estimate a delivery fee.',
      'How does SwiftDrop work?',
      'What does IN_TRANSIT mean?',
    ]);
  });

  it('gives each signed-in role capabilities its backend tools support', () => {
    expect(suggestedQuestionsFor('CUSTOMER')).toContain('Track my parcel.');
    expect(suggestedQuestionsFor('CUSTOMER')).toContain(
      'Show my active parcels.',
    );
    expect(suggestedQuestionsFor('CUSTOMER')).not.toContain(
      'Check courier workloads.',
    );

    expect(suggestedQuestionsFor('COURIER')).toEqual([
      'Show my active deliveries.',
      'Show my completed deliveries.',
      'Do I have any failed deliveries?',
      'Find a delivery by parcel ID.',
    ]);
    expect(suggestedQuestionsFor('COURIER')).not.toContain(
      'Show unassigned parcels.',
    );

    expect(suggestedQuestionsFor('MERCHANT')).toContain(
      'Show my recent orders.',
    );
    expect(suggestedQuestionsFor('MERCHANT')).toContain(
      'Show my webhook delivery failures.',
    );
    expect(suggestedQuestionsFor('MERCHANT')).not.toContain('Track my parcel.');

    expect(suggestedQuestionsFor('DISPATCHER')).toEqual([
      'Show unassigned parcels.',
      'Check courier workloads.',
      'Search for a parcel.',
      'Show failed deliveries.',
    ]);
    expect(suggestedQuestionsFor('ADMIN')).toEqual([
      'Show unassigned parcels.',
      'Search parcels.',
      'Check courier workloads.',
      'View parcel details.',
    ]);
  });

  it('uses plain strings that fit the chat limit', () => {
    for (const role of [
      null,
      'CUSTOMER',
      'COURIER',
      'MERCHANT',
      'DISPATCHER',
      'ADMIN',
    ] as const) {
      for (const question of suggestedQuestionsFor(role)) {
        expect(question.length).toBeLessThanOrEqual(120);
      }
    }
  });
});
