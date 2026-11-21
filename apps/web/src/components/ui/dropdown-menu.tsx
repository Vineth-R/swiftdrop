'use client';

import * as React from 'react';
import { cn } from '../../lib/utils';
import { CheckIcon, ChevronRightIcon, CircleIcon } from 'lucide-react';
import { DropdownMenu as DropdownMenuPrimitive } from 'radix-ui';

type KeyboardOpen = {
  /** Set by the trigger so the content knows how the menu was opened. */
  isKeyboardOpenRef: React.MutableRefObject<boolean>;
};

const KeyboardOpenContext = React.createContext<KeyboardOpen | null>(null);

function DropdownMenu({
  onOpenChange,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Root>) {
  const isKeyboardOpenRef = React.useRef(false);
  const handleOpenChange = React.useCallback(
    (open: boolean) => {
      if (!open) isKeyboardOpenRef.current = false;
      onOpenChange?.(open);
    },
    [onOpenChange],
  );
  const context = React.useMemo(() => ({ isKeyboardOpenRef }), []);

  return (
    <KeyboardOpenContext.Provider value={context}>
      <DropdownMenuPrimitive.Root
        data-slot="dropdown-menu"
        {...props}
        onOpenChange={handleOpenChange}
      />
    </KeyboardOpenContext.Provider>
  );
}

function DropdownMenuPortal({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Portal>) {
  return (
    <DropdownMenuPrimitive.Portal data-slot="dropdown-menu-portal" {...props} />
  );
}

function DropdownMenuTrigger({
  onKeyDown,
  onPointerDown,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Trigger>) {
  const context = React.useContext(KeyboardOpenContext);

  return (
    <DropdownMenuPrimitive.Trigger
      data-slot="dropdown-menu-trigger"
      {...props}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        // Radix normally tracks this with a document listener added in an
        // effect. Recording it at the trigger instead keeps the first open
        // after a slow page load keyboard-aware, before that effect runs.
        if (['Enter', ' ', 'ArrowDown'].includes(event.key)) {
          if (context) context.isKeyboardOpenRef.current = true;
        }
      }}
      onPointerDown={(event) => {
        onPointerDown?.(event);
        if (context) context.isKeyboardOpenRef.current = false;
      }}
    />
  );
}

type DropdownMenuContentProps = React.ComponentProps<
  typeof DropdownMenuPrimitive.Content
> & {
  /** Radix forwards this to the content's FocusScope at runtime. */
  onOpenAutoFocus?: (event: Event) => void;
};

function DropdownMenuContent({
  className,
  sideOffset = 4,
  onOpenAutoFocus,
  ...props
}: DropdownMenuContentProps) {
  const context = React.useContext(KeyboardOpenContext);

  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        data-slot="dropdown-menu-content"
        sideOffset={sideOffset}
        // Radix forwards onOpenAutoFocus to its FocusScope at runtime, though
        // its public content type omits it.
        {...({
          onOpenAutoFocus: (event: Event) => {
            onOpenAutoFocus?.(event);
            if (event.defaultPrevented || !context?.isKeyboardOpenRef.current) {
              return;
            }
            const content = event.currentTarget as HTMLElement | null;
            const firstItem = content?.querySelector<HTMLElement>(
              '[data-radix-collection-item]:not([data-disabled])',
            );
            if (!firstItem) return;
            // Take focus ourselves so Radix's entry-focus pass cannot run with
            // a stale keyboard-modality ref and leave focus on the content.
            event.preventDefault();
            firstItem.focus({ preventScroll: true });
          },
        } as React.ComponentProps<typeof DropdownMenuPrimitive.Content>)}
        className={cn(
          'z-50 max-h-(--radix-dropdown-menu-content-available-height) min-w-32 origin-(--radix-dropdown-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-card border border-line bg-popover p-1 text-popover-foreground shadow-popover data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
          className,
        )}
        {...props}
      />
    </DropdownMenuPrimitive.Portal>
  );
}

function DropdownMenuGroup({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Group>) {
  return (
    <DropdownMenuPrimitive.Group data-slot="dropdown-menu-group" {...props} />
  );
}

function DropdownMenuItem({
  className,
  inset,
  variant = 'default',
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Item> & {
  inset?: boolean;
  variant?: 'default' | 'destructive';
}) {
  return (
    <DropdownMenuPrimitive.Item
      data-slot="dropdown-menu-item"
      data-inset={inset}
      data-variant={variant}
      className={cn(
        "relative flex h-10 cursor-default items-center gap-3 rounded-chip px-3 text-button font-medium text-text outline-hidden select-none focus:bg-surface-2 focus:text-text data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[inset]:pl-8 data-[variant=destructive]:text-destructive data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive dark:data-[variant=destructive]:focus:bg-destructive/20 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground data-[variant=destructive]:*:[svg]:text-destructive!",
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuCheckboxItem({
  className,
  children,
  checked,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>) {
  return (
    <DropdownMenuPrimitive.CheckboxItem
      data-slot="dropdown-menu-checkbox-item"
      className={cn(
        "relative flex cursor-default items-center gap-2 rounded-chip py-2 pr-2 pl-8 text-body-sm outline-hidden select-none focus:bg-surface-2 focus:text-text data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      checked={checked}
      {...props}
    >
      <span className="pointer-events-none absolute left-2 flex size-4 items-center justify-center">
        <DropdownMenuPrimitive.ItemIndicator>
          <CheckIcon className="size-4" />
        </DropdownMenuPrimitive.ItemIndicator>
      </span>
      {children}
    </DropdownMenuPrimitive.CheckboxItem>
  );
}

function DropdownMenuRadioGroup({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.RadioGroup>) {
  return (
    <DropdownMenuPrimitive.RadioGroup
      data-slot="dropdown-menu-radio-group"
      {...props}
    />
  );
}

function DropdownMenuRadioItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.RadioItem>) {
  return (
    <DropdownMenuPrimitive.RadioItem
      data-slot="dropdown-menu-radio-item"
      className={cn(
        "relative flex cursor-default items-center gap-2 rounded-chip py-2 pr-2 pl-8 text-body-sm outline-hidden select-none focus:bg-surface-2 focus:text-text data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    >
      <span className="pointer-events-none absolute left-2 flex size-4 items-center justify-center">
        <DropdownMenuPrimitive.ItemIndicator>
          <CircleIcon className="size-2 fill-current" />
        </DropdownMenuPrimitive.ItemIndicator>
      </span>
      {children}
    </DropdownMenuPrimitive.RadioItem>
  );
}

function DropdownMenuLabel({
  className,
  inset,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Label> & {
  inset?: boolean;
}) {
  return (
    <DropdownMenuPrimitive.Label
      data-slot="dropdown-menu-label"
      data-inset={inset}
      className={cn(
        'px-3 py-2 text-label font-semibold data-[inset]:pl-8',
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuSeparator({
  className,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Separator>) {
  return (
    <DropdownMenuPrimitive.Separator
      data-slot="dropdown-menu-separator"
      className={cn('-mx-1 my-1 h-px bg-border', className)}
      {...props}
    />
  );
}

function DropdownMenuShortcut({
  className,
  ...props
}: React.ComponentProps<'span'>) {
  return (
    <span
      data-slot="dropdown-menu-shortcut"
      className={cn(
        'ml-auto text-caption tracking-widest text-muted-foreground',
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuSub({
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Sub>) {
  return <DropdownMenuPrimitive.Sub data-slot="dropdown-menu-sub" {...props} />;
}

function DropdownMenuSubTrigger({
  className,
  inset,
  children,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.SubTrigger> & {
  inset?: boolean;
}) {
  return (
    <DropdownMenuPrimitive.SubTrigger
      data-slot="dropdown-menu-sub-trigger"
      data-inset={inset}
      className={cn(
        "flex cursor-default items-center gap-2 rounded-chip px-2 py-2 text-body-sm outline-hidden select-none focus:bg-surface-2 focus:text-text data-[inset]:pl-8 data-[state=open]:bg-surface-2 data-[state=open]:text-text [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground",
        className,
      )}
      {...props}
    >
      {children}
      <ChevronRightIcon className="ml-auto size-4" />
    </DropdownMenuPrimitive.SubTrigger>
  );
}

function DropdownMenuSubContent({
  className,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.SubContent>) {
  return (
    <DropdownMenuPrimitive.SubContent
      data-slot="dropdown-menu-sub-content"
      className={cn(
        'z-50 min-w-32 origin-(--radix-dropdown-menu-content-transform-origin) overflow-hidden rounded-card border border-line bg-popover p-1 text-popover-foreground shadow-popover data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
        className,
      )}
      {...props}
    />
  );
}

export {
  DropdownMenu,
  DropdownMenuPortal,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
};
