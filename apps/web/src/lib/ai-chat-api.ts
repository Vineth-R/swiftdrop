import { api } from './api';
import type { Role } from './tokens';
import { ApiError } from './api-errors';

/**
 * The SwiftDrop Delivery Assistant API (API v1.1, `POST /ai/chat`).
 *
 * The browser never talks to Gemini. It sends a message and, on every
 * follow-up, the conversation ID the assistant returned, through the same
 * cookie-authenticated `/api/v1` client as the rest of the app. Signed-in
 * callers keep their session cookie, so private tools stay behind the
 * backend's own authorization checks.
 */

/** The contract's hard limit on a chat message, in Unicode characters. */
export const CHAT_MESSAGE_MAX = 500;

/**
 * Starting questions shown before the first message. Each set only suggests
 * capabilities the backend exposes to that role; the assistant still decides
 * whether any tool may run, and a suggestion is never an authorization grant.
 */
export const SUGGESTED_QUESTIONS: Record<Role | 'GUEST', readonly string[]> = {
  GUEST: [
    'Track a parcel.',
    'Estimate a delivery fee.',
    'How does SwiftDrop work?',
    'What does IN_TRANSIT mean?',
  ],
  CUSTOMER: [
    'Track my parcel.',
    'Show my active parcels.',
    'View my parcel history.',
    'How much would a delivery cost?',
  ],
  COURIER: [
    'Show my active deliveries.',
    'Show my completed deliveries.',
    'Do I have any failed deliveries?',
    'Find a delivery by parcel ID.',
  ],
  MERCHANT: [
    'Show my recent orders.',
    'Track an order.',
    'Show my webhook delivery failures.',
    'Which of my orders are still pending?',
  ],
  DISPATCHER: [
    'Show unassigned parcels.',
    'Check courier workloads.',
    'Search for a parcel.',
    'Show failed deliveries.',
  ],
  ADMIN: [
    'Show unassigned parcels.',
    'Search parcels.',
    'Check courier workloads.',
    'View parcel details.',
  ],
};

/** The suggested questions for a verified role, or the guest set for null. */
export function suggestedQuestionsFor(role: Role | null): readonly string[] {
  return role ? SUGGESTED_QUESTIONS[role] : SUGGESTED_QUESTIONS.GUEST;
}

/** The safe outcome of one approved tool run, never its payload. */
export type ChatToolStatus = 'OK' | 'INVALID' | 'DENIED' | 'UNAVAILABLE';

export type ChatToolCall = {
  name: string;
  status: ChatToolStatus;
};

export type ChatReply = {
  conversationId: string;
  reply: string;
  toolCalls: ChatToolCall[];
};

const CONVERSATION_ID = /^cv[0-9a-f]{32}$/;

const TOOL_STATUSES: ReadonlySet<string> = new Set([
  'OK',
  'INVALID',
  'DENIED',
  'UNAVAILABLE',
]);

const TOOL_LABELS: Record<string, string> = {
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

/**
 * Sends one turn. `conversationId` is null for a new conversation and the
 * value from the previous reply afterwards. The caller's AbortSignal cancels
 * the request when the panel closes or the person starts again.
 */
export async function sendChatMessage(
  input: { message: string; conversationId: string | null },
  signal?: AbortSignal,
): Promise<ChatReply> {
  const data = await api<ChatReply>('/ai/chat', {
    method: 'POST',
    body: {
      message: input.message.trim(),
      conversationId: input.conversationId,
    },
    signal,
  });
  return validateReply(data);
}

/**
 * A response we cannot understand is a fault on our side, not something to
 * render. It becomes the same INTERNAL presentation as a 5xx.
 */
function validateReply(data: unknown): ChatReply {
  if (!data || typeof data !== 'object') throw malformedReply();
  const candidate = data as Partial<ChatReply>;
  if (
    typeof candidate.conversationId !== 'string' ||
    !CONVERSATION_ID.test(candidate.conversationId) ||
    typeof candidate.reply !== 'string' ||
    !Array.isArray(candidate.toolCalls)
  ) {
    throw malformedReply();
  }
  const toolCalls = candidate.toolCalls.filter(
    (call): call is ChatToolCall =>
      Boolean(call) &&
      typeof call === 'object' &&
      typeof (call as ChatToolCall).name === 'string' &&
      TOOL_STATUSES.has((call as ChatToolCall).status),
  );
  return {
    conversationId: candidate.conversationId,
    reply: candidate.reply,
    toolCalls,
  };
}

function malformedReply(): ApiError {
  return new ApiError({ code: 'INTERNAL', status: 502 });
}

/** True when the conversation expired or belongs to someone else. */
export function isConversationGone(error: unknown): boolean {
  return error instanceof ApiError && error.code === 'CONVERSATION_NOT_FOUND';
}

/**
 * What to show for a failed turn. The API's own message is never displayed;
 * only these sentences are. `canRetry` is false when sending the same message
 * again cannot help (a 422, a 401, or a rate limit that must expire first).
 */
export type ChatFailure = {
  title: string;
  message: string;
  canRetry: boolean;
  /** Present on a 429 so the panel can say how long to wait. */
  waitSeconds?: number;
  /** Present on our own 5xx faults, matching the rest of the app. */
  reference?: string;
};

export function describeChatFailure(error: unknown): ChatFailure {
  if (!(error instanceof ApiError)) {
    return {
      title: 'Something broke on our side',
      message: 'Please try again.',
      canRetry: true,
    };
  }

  switch (error.code) {
    case 'RATE_LIMITED':
      return {
        title: 'Too many messages',
        message:
          error.retryAfter && error.retryAfter > 0
            ? `Please wait ${error.retryAfter} seconds and try again.`
            : 'Please wait a moment and try again.',
        canRetry: false,
        waitSeconds: error.retryAfter,
      };
    case 'AI_UNAVAILABLE':
      return {
        title: 'The assistant is unavailable',
        message:
          'The assistant is temporarily unavailable. Please try again later.',
        canRetry: true,
      };
    case 'VALIDATION_FAILED':
      return {
        title: 'That message could not be sent',
        message: 'Check the message and try again.',
        canRetry: false,
      };
    case 'UNAUTHENTICATED':
      return {
        title: 'Sign in to continue',
        message:
          'Sign in to see your own parcels and keep this conversation going.',
        canRetry: false,
      };
    case 'NETWORK':
      return {
        title: 'We could not reach SwiftDrop',
        message: 'Check your connection and try again.',
        canRetry: true,
      };
    default:
      return {
        title: 'Something broke on our side',
        message: 'Please try again.',
        canRetry: true,
        ...(error.isServerFault ? { reference: error.requestId } : {}),
      };
  }
}

/**
 * A short, safe label for a tool the assistant used. Only failures need a
 * label: an OK call is already explained in the reply, and tool data is never
 * rendered.
 */
export function toolProgressLabel(call: ChatToolCall): string | null {
  const subject = TOOL_LABELS[call.name];
  if (!subject) return null;
  switch (call.status) {
    case 'DENIED':
      return `That information is not available with your current access (${subject}).`;
    case 'UNAVAILABLE':
      return `${capitalise(subject)} is temporarily unavailable.`;
    case 'INVALID':
      return `More details are needed for ${subject}.`;
    default:
      return null;
  }
}

function capitalise(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
