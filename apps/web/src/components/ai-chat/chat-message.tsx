import { Sparkles } from 'lucide-react';

import { toolProgressLabel, type ChatToolCall } from '../../lib/ai-chat-api';
import { cn } from '../../lib/cn';

type ChatMessageProps = {
  role: 'user' | 'assistant';
  text: string;
  toolCalls?: ChatToolCall[];
};

/**
 * One turn in the conversation. A user message is the Ink fill; the assistant
 * answers on a quiet surface. Tool calls are reduced to a short, safe note
 * ("Sign in to use your active deliveries"); their data is never rendered.
 */
export function ChatMessage({ role, text, toolCalls }: ChatMessageProps) {
  const isUser = role === 'user';
  const notes = (toolCalls ?? [])
    .map(toolProgressLabel)
    .filter((note): note is string => note !== null);

  return (
    <div className={cn('flex gap-2', isUser ? 'justify-end' : 'justify-start')}>
      {!isUser && (
        <span
          aria-hidden="true"
          className="mt-1 inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-primary text-on-primary"
        >
          <Sparkles size={14} strokeWidth={1.5} />
        </span>
      )}
      <div
        className={cn(
          'flex max-w-md flex-col gap-1 rounded-card px-3 py-2 text-body-sm',
          isUser
            ? 'bg-primary text-on-primary'
            : 'border border-line bg-surface-2 text-text',
        )}
      >
        <p className="whitespace-pre-wrap break-words">{text}</p>
        {notes.length > 0 && (
          <p
            className={cn(
              'text-caption',
              isUser ? 'text-on-primary' : 'text-text-muted',
            )}
          >
            {notes.join(' ')}
          </p>
        )}
      </div>
    </div>
  );
}
