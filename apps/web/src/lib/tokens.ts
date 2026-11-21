/**
 * Token names as typed unions, so props such as `tone` or `variant` are
 * checked by the compiler instead of being free strings. The values live in
 * app/globals.css; this file only lists names. Keep the two in step.
 */

export const colorTokens = {
  'Ground and ink': [
    'ink',
    'paper',
    'surface',
    'surface-2',
    'surface-3',
    'line',
    'line-strong',
    'line-control',
  ],
  Text: ['text', 'text-muted', 'text-faint', 'text-disabled'],
  Actions: [
    'primary',
    'primary-hover',
    'primary-active',
    'on-primary',
    'accent',
    'accent-hover',
    'accent-active',
    'on-accent',
    'interactive',
    'interactive-hover',
    'interactive-tint',
    'interactive-border',
  ],
  Semantic: [
    'success',
    'success-tint',
    'success-border',
    'warning',
    'warning-tint',
    'warning-border',
    'danger',
    'danger-tint',
    'danger-border',
    'danger-hover',
    'danger-active',
    'on-danger',
    'info',
    'info-tint',
    'info-border',
  ],
  'Parcel status': [
    'status-created-fg',
    'status-created-bg',
    'status-created-bd',
    'status-picked-up-fg',
    'status-picked-up-bg',
    'status-picked-up-bd',
    'status-in-transit-fg',
    'status-in-transit-bg',
    'status-in-transit-bd',
    'status-out-fg',
    'status-out-bg',
    'status-out-bd',
    'status-delivered-fg',
    'status-delivered-bg',
    'status-delivered-bd',
    'status-failed-fg',
    'status-failed-bg',
    'status-failed-bd',
  ],
} as const;

type ColorGroups = typeof colorTokens;
export type ColorToken = ColorGroups[keyof ColorGroups][number];

/** Written out in full so Tailwind's scanner sees every class. */
export const textScale = [
  {
    token: 'display',
    className: 'font-display text-display',
    spec: '56/60 · 700',
  },
  { token: 'h1', className: 'font-display text-h1', spec: '36/42 · 600' },
  { token: 'h2', className: 'font-display text-h2', spec: '28/34 · 600' },
  { token: 'h3', className: 'text-h3', spec: '22/28 · 600' },
  { token: 'h4', className: 'text-h4', spec: '18/24 · 600' },
  { token: 'body', className: 'text-body', spec: '15/23 · 400' },
  { token: 'body-sm', className: 'text-body-sm', spec: '13/20 · 400' },
  { token: 'label', className: 'text-label', spec: '13/16 · 500' },
  { token: 'caption', className: 'text-caption', spec: '12/16 · 400' },
  {
    token: 'overline',
    className: 'text-overline uppercase',
    spec: '11/14 · 600 · caps',
  },
] as const;

export type TextToken = (typeof textScale)[number]['token'];

export const spaceScale = [1, 2, 3, 4, 5, 6, 8, 10, 12, 16, 20, 24] as const;
export type SpaceToken = (typeof spaceScale)[number];

export const radiusTokens = ['chip', 'control', 'card', 'splash'] as const;
export type RadiusToken = (typeof radiusTokens)[number];

export const shadowTokens = ['popover', 'modal'] as const;
export type ShadowToken = (typeof shadowTokens)[number];

export const durationTokens = ['fast', 'base', 'slow'] as const;
export type DurationToken = (typeof durationTokens)[number];

/** Parcel statuses as the API names them. */
export const parcelStatuses = [
  'CREATED',
  'PICKED_UP',
  'IN_TRANSIT',
  'OUT_FOR_DELIVERY',
  'DELIVERED',
  'FAILED',
] as const;
export type ParcelStatus = (typeof parcelStatuses)[number];

export const roles = [
  'CUSTOMER',
  'MERCHANT',
  'COURIER',
  'DISPATCHER',
  'ADMIN',
] as const;
export type Role = (typeof roles)[number];
