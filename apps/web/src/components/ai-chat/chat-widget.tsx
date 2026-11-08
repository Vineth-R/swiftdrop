'use client';

import { MessageCircle } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

import { ChatPanel } from './chat-panel';

/**
 * The floating launcher and its panel. It sits outside the app shell's layout
 * flow, so it is available on every page: public ones stay anonymous, while a
 * signed-in page's session cookie lets the backend use private tools.
 */
export function AiChatWidget() {
  const [isOpen, setIsOpen] = useState(false);
  const launcherRef = useRef<HTMLButtonElement>(null);
  const hasOpened = useRef(false);

  useEffect(() => {
    if (!isOpen && hasOpened.current) launcherRef.current?.focus();
  }, [isOpen]);

  function open() {
    hasOpened.current = true;
    setIsOpen(true);
  }

  return (
    <>
      {isOpen && <ChatPanel onClose={() => setIsOpen(false)} />}
      <button
        ref={launcherRef}
        type="button"
        aria-expanded={isOpen}
        aria-controls={isOpen ? 'ai-chat-panel' : undefined}
        aria-label={
          isOpen
            ? 'Close the SwiftDrop Delivery Assistant'
            : 'Open the SwiftDrop Delivery Assistant'
        }
        onClick={() => (isOpen ? setIsOpen(false) : open())}
        className="fixed bottom-20 right-4 z-40 inline-flex size-14 items-center justify-center rounded-full border border-primary bg-primary text-on-primary shadow-popover transition-colors duration-fast ease-out-sd hover:bg-primary-hover active:bg-primary-active"
      >
        <MessageCircle size={24} strokeWidth={1.5} aria-hidden="true" />
      </button>
    </>
  );
}
