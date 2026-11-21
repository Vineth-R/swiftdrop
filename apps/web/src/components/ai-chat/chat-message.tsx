import { Sparkles } from 'lucide-react';
import Link from 'next/link';
import Markdown, { type Components } from 'react-markdown';

import { toolProgressLabel, type ChatToolCall } from '../../lib/ai-chat-api';
import { cn } from '../../lib/cn';

type ChatMessageProps = {
  role: 'user' | 'assistant';
  text: string;
  toolCalls?: ChatToolCall[];
};

const SAFE_SCHEMES = ['http:', 'https:', 'mailto:'];

function safeUrl(url: string): string {
  const trimmed = url.trim();
  if (trimmed.startsWith('#')) return trimmed;
  if (trimmed.startsWith('/') && !trimmed.startsWith('//')) return trimmed;
  try {
    const parsed = new URL(trimmed);
    if (SAFE_SCHEMES.includes(parsed.protocol)) return trimmed;
  } catch {
    // Not an absolute URL. Relative links must start with "/" or "#".
  }
  return '';
}

const markdownComponents: Components = {
  p: ({ children }) => (
    <p className="whitespace-pre-wrap break-words">{children}</p>
  ),
  strong: ({ children }) => (
    <strong className="font-semibold">{children}</strong>
  ),
  em: ({ children }) => <em className="italic">{children}</em>,
  ul: ({ children }) => (
    <ul className="my-2 list-disc space-y-1 pl-5">{children}</ul>
  ),
  ol: ({ children }) => (
    <ol className="my-2 list-decimal space-y-1 pl-5">{children}</ol>
  ),
  li: ({ children }) => <li className="break-words">{children}</li>,
  code: ({ children }) => (
    <code className="rounded-control bg-surface-2 px-1 font-mono text-caption">
      {children}
    </code>
  ),
  pre: ({ children }) => (
    <pre className="my-2 max-w-full overflow-x-auto rounded-control bg-surface-2 p-2 font-mono text-caption">
      {children}
    </pre>
  ),
  a: ({ href, children }) => {
    const url = href ?? '';
    if (url.startsWith('/') || url.startsWith('#')) {
      return (
        <a
          href={url}
          className="underline decoration-line underline-offset-2 hover:text-text"
        >
          {children}
        </a>
      );
    }
    return (
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer nofollow"
        className="underline decoration-line underline-offset-2 hover:text-text"
      >
        {children}
      </a>
    );
  },
  blockquote: ({ children }) => (
    <blockquote className="my-2 border-l-2 border-line pl-3 text-text-muted">
      {children}
    </blockquote>
  ),
  h1: ({ children }) => <p className="font-semibold">{children}</p>,
  h2: ({ children }) => <p className="font-semibold">{children}</p>,
  h3: ({ children }) => <p className="font-semibold">{children}</p>,
};

/**
 * One turn in the conversation. A user message is the Ink fill; the assistant
 * answers on a quiet surface. Assistant replies are rendered as a safe Markdown
 * subset (no raw HTML). Tool calls are reduced to a short, safe note
 * ("Sign in to use your active deliveries"); their data is never rendered.
 */
export function ChatMessage({ role, text, toolCalls }: ChatMessageProps) {
  const isUser = role === 'user';
  const notes = (toolCalls ?? [])
    .map(toolProgressLabel)
    .filter((note): note is string => note !== null);
  const denied = (toolCalls ?? []).some((call) => call.status === 'DENIED');

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
        {isUser ? (
          <p className="whitespace-pre-wrap break-words">{text}</p>
        ) : (
          <div className="flex flex-col gap-1 break-words">
            <Markdown
              components={markdownComponents}
              allowedElements={[
                'p',
                'strong',
                'em',
                'ul',
                'ol',
                'li',
                'code',
                'pre',
                'a',
                'blockquote',
                'br',
                'h1',
                'h2',
                'h3',
              ]}
              urlTransform={safeUrl}
            >
              {text}
            </Markdown>
          </div>
        )}
        {notes.length > 0 && (
          <p
            className={cn(
              'text-caption',
              isUser ? 'text-on-primary' : 'text-text-muted',
            )}
          >
            {notes.join(' ')}
            {denied && (
              <>
                {' '}
                <Link
                  href="/login"
                  className="underline decoration-line underline-offset-2 hover:text-text"
                >
                  Sign in
                </Link>
              </>
            )}
          </p>
        )}
      </div>
    </div>
  );
}
