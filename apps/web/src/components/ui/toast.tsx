'use client';

import { X } from 'lucide-react';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';

import { cn } from '../../lib/cn';
import { alertIcons, alertToneClasses, type AlertTone } from './alert';

export type ToastOptions = {
  /** What happened, in one short sentence: "Details saved." */
  title: string;
  /** What to do next, if there is something. */
  description?: string;
  /** Defaults to success. */
  tone?: AlertTone;
  /** Milliseconds on screen. 0 keeps it until dismissed. Default 5000. */
  duration?: number;
};

type ToastEntry = ToastOptions & { id: number };

type ToastApi = {
  /** Shows a toast and returns its id. */
  show: (options: ToastOptions) => number;
  dismiss: (id: number) => void;
};

const ToastContext = createContext<ToastApi | null>(null);

// A toast is a white card, so only its border takes the tone's colour.
const borders: Record<AlertTone, string> = {
  info: 'border-info-border',
  success: 'border-success-border',
  warning: 'border-warning-border',
  danger: 'border-danger-border',
};

/** The most toasts on screen at once. The oldest leaves first. */
const maxVisible = 3;
const defaultDuration = 5000;

/** Call `show` from an event handler, after an action succeeded. */
export function useToast(): ToastApi {
  const api = useContext(ToastContext);
  if (!api) throw new Error('useToast needs a ToastProvider above it.');
  return api;
}

/**
 * Keeps the toasts and draws them in the corner. It is mounted once, in the
 * root layout, so every screen can confirm an action without a place on the
 * page of its own. Hovering or focusing a toast pauses its timer.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastEntry[]>([]);
  const nextId = useRef(1);

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const show = useCallback((options: ToastOptions) => {
    const id = nextId.current++;
    setToasts((current) => [...current, { ...options, id }].slice(-maxVisible));
    return id;
  }, []);

  const api = useMemo(() => ({ show, dismiss }), [show, dismiss]);

  return (
    <ToastContext value={api}>
      {children}
      <section
        aria-label="Notifications"
        className="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex flex-col items-center gap-3 p-4 md:items-end"
      >
        {toasts.map((toast) => (
          <ToastItem key={toast.id} toast={toast} onDismiss={dismiss} />
        ))}
      </section>
    </ToastContext>
  );
}

function ToastItem({
  toast,
  onDismiss,
}: {
  toast: ToastEntry;
  onDismiss: (id: number) => void;
}) {
  const { id, title, description, tone = 'success' } = toast;
  const duration = toast.duration ?? defaultDuration;
  const [isPaused, setIsPaused] = useState(false);
  const Icon = alertIcons[tone];
  const look = alertToneClasses[tone];

  useEffect(() => {
    if (duration === 0 || isPaused) return;
    const timer = setTimeout(() => onDismiss(id), duration);
    return () => clearTimeout(timer);
  }, [id, duration, isPaused, onDismiss]);

  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
      onFocus={() => setIsPaused(true)}
      onBlur={() => setIsPaused(false)}
      className={cn(
        'pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-card border bg-surface p-4 shadow-popover',
        borders[tone],
      )}
    >
      <Icon
        size={20}
        strokeWidth={1.5}
        aria-hidden="true"
        className={cn('mt-0.5 shrink-0', look.icon)}
      />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-label">{title}</p>
        {description && (
          <p className="text-body-sm text-text-muted">{description}</p>
        )}
      </div>
      <button
        type="button"
        aria-label="Dismiss notification"
        onClick={() => onDismiss(id)}
        className="-mr-2 -mt-1 inline-flex size-8 pointer-coarse:size-11 shrink-0 items-center justify-center rounded-control text-text-muted hover:bg-surface-2"
      >
        <X size={16} strokeWidth={1.5} aria-hidden="true" />
      </button>
    </div>
  );
}
