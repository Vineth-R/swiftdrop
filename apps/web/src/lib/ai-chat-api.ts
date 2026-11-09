import { api } from './api';
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

/** Starting questions shown before the first message. */
export const SUGGESTED_QUESTIONS = [
  'What is SwiftDrop?',
  'How does delivery pricing work?',
  'Track my parcel.',
  'Show my active deliveries.',
  'What are your terms and conditions?',
] as const;

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
  get_my_parcel_history: 'your parcel history',
  get_my_active_deliveries: 'your active deliveries',
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
      return `Sign in or use a customer account to use ${subject}.`;
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
