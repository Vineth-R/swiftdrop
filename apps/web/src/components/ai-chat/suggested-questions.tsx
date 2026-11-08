import { ArrowUpRight } from 'lucide-react';

import { SUGGESTED_QUESTIONS } from '../../lib/ai-chat-api';
import { cn } from '../../lib/cn';
import { buttonClasses } from '../ui/button';

type SuggestedQuestionsProps = {
  onSelect: (question: string) => void;
  isDisabled?: boolean;
};

/**
 * Starting points, not canned answers: each one sends a real message and the
 * reply comes back from the assistant.
 */
export function SuggestedQuestions({
  onSelect,
  isDisabled = false,
}: SuggestedQuestionsProps) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-overline uppercase text-text-muted">Try asking</p>
      <ul aria-label="Suggested questions" className="flex flex-col gap-2">
        {SUGGESTED_QUESTIONS.map((question) => (
          <li key={question}>
            <button
              type="button"
              disabled={isDisabled}
              onClick={() => onSelect(question)}
              className={cn(
                buttonClasses({ variant: 'secondary', size: 'sm' }),
                'h-auto min-h-11 w-full justify-between gap-3 whitespace-normal py-2 text-left',
              )}
            >
              <span>{question}</span>
              <ArrowUpRight
                size={16}
                strokeWidth={1.5}
                aria-hidden="true"
                className="shrink-0 text-text-muted"
              />
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
