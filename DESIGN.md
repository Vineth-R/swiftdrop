# SwiftDrop frontend design guide

**Read this before you write or change any code in `apps/web`.** It is the contract for how SwiftDrop screens look and behave. It applies to people and to AI coding agents alike.

The web foundation (design tokens, shadcn/ui primitives, UI kit, app shell, API client, form tools and test helpers) is owned by M2. It already exists. Your job is to build a screen's own content on top of it, not to rebuild or restyle it.

## 0. For AI agents: how to use this file

1. Read the whole file before you plan a screen. Do not skim it.
2. Build with what exists (section 7). Do not create look-alike components, new colours, new fonts, new icon sets, new spacing values or new libraries.
3. When a request would break a rule in this file, do not break it quietly. Build the closest version that follows the rules and say what you left out and why, or ask.
4. When you need something that does not exist (section 8), stop and say so. Leave a `// TODO(design-foundation)` comment and a plain placeholder. Do not invent a one-off.
5. Do not edit the protected files (section 14) unless the task is explicitly about the design foundation.
6. Finish with the checklist in section 15, run lint (section 16), and report any item you could not satisfy.

Sources of truth, in this order when they disagree:

1. This file.
2. `apps/web/src/app/globals.css`, which holds every token value. Never copy a value out of it.
3. The running style guide at `/design` (`pnpm exec nx dev @org/web`, then open `http://localhost:3000/design`).
4. The design boards the team shares outside the repo. Treat a board as a specification: copy its layout, spacing, states and wording, and build it with our components. Never paste its exported HTML or inline styles.
5. `docs/API_Documentation_v1.1.md` and `contracts/openapi/*.yaml` for every API shape.
6. `AGENTS.md` for the repo-wide rules.

## 1. The hard rules

Most of the **Never** list is checked by lint (section 16): a violation fails `nx lint` and CI, so do not rely on review to catch it.

**Always**

- Put every signed-in screen at `apps/web/src/app/(app)/<route>/page.tsx`. That route group already gives it the session check and the role's shell.
- Start every page with `PageHeader`, then `Section` blocks. Do not add outer padding or a max width to a page: the shell does it.
- Compose screens from `components/ui` and `components/layout` (section 7). The shadcn wrappers in `components/ui` are the only place that may import Radix.
- Style with token classes only (section 4).
- Call the API only through `api()` or `apiWithMeta()` from `lib/api.ts`.
- Design the empty, loading, error and no-permission state of every screen, not only the happy path.
- Check the screen at 375px wide, in dark mode, and with the keyboard only.
- Write Playwright tests for the screen, using `signInAs` and `mockApi` so no backend is needed.

**Never**

- Write a hex, `rgb()` or `hsl()` colour, an arbitrary Tailwind value (`p-[13px]`, `text-[#fff]`, `w-[317px]`) or an inline `style` for how something looks.
- Use a default Tailwind colour, type, radius or shadow utility in a screen (`bg-red-500`, `text-sm`, `rounded-lg`, `shadow-md`). The foundation maps a small set of shadcn semantic aliases (`rounded-md`, `rounded-lg`, `shadow-md`, `shadow-lg`) to SwiftDrop tokens for generated primitives only; screens must use the SwiftDrop token classes in section 4.
- Use a spacing step outside the scale in section 4 (`p-7`, `gap-9`). It compiles, but it breaks the design.
- Put a shadow on a card, table, sidebar or top bar. Only popovers and modals have one.
- Add another UI library (shadcn/ui is the approved foundation), a state-management library, CSS-in-JS, another icon set or another font.
- Call `fetch` directly, call a service port, or show `error.message` or any server text to a person.
- Use `bg-ink` or `text-paper` on an ordinary surface. They do not flip in dark mode (section 3).
- Edit `app/(app)/layout.tsx`, `app/globals.css` or the kit's components to suit one screen.
- Invent a sixth layout, a seventh parcel status or a second primary button.
- Show names, phone numbers, addresses or fees on a page that needs no login.

## 2. The twelve design language rules

These come from the design system's "Design language rules" board. A new screen that passes all twelve looks like it belongs, whoever builds it. Read them before you start and again before you open a pull request.

| #   | Rule                                            | What it means here                                                                                                                                                                                                                                                                             |
| --- | ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 01  | One primary action per view                     | Exactly one `Button` with `variant="primary"` (the Ink fill). Everything else is `secondary`, `ghost` or `link`. If two actions both feel primary, the screen is doing two jobs. The `accent` variant is for marketing pages only.                                                             |
| 02  | Hairlines before shadows                        | Separate things with a 1px line (`border border-line`). Only popovers (`shadow-popover`) and modals (`shadow-modal`) carry a shadow. Cards, tables, the sidebar and the top bar never do.                                                                                                      |
| 03  | Mono for identifiers, never for emphasis        | Tracking numbers, IDs, keys, request IDs, amounts and weights use `font-mono` (with `tabular` for columns of numbers). Names, cities and labels stay in the default font. Amounts are right-aligned with two decimals: `LKR 550.00`.                                                           |
| 04  | A status always has a label and a shape         | Never a bare coloured dot. A status is a chip with its name in it. If space is very tight, the dot carries a `title` and visually hidden text with the status name.                                                                                                                            |
| 05  | The accent appears at most once per screen area | Lime marks the active nav item, a progress fill, the brand mark, or one marketing call to action. Never two of those in the same region, and never as text colour.                                                                                                                             |
| 06  | Tokens only, no raw values                      | Every colour, size, space, radius and duration comes from the token set. A hex value or a `14px` in a component is a bug, not a decision. The Tailwind reset in `globals.css` enforces this: stray defaults silently do nothing (arbitrary values still compile, so review has to catch them). |
| 07  | Four states or it is not finished               | Every screen has an empty, a loading, an error and a no-permission state designed, not just the happy path. After an action, confirm the result in plain words.                                                                                                                                |
| 08  | Nothing scrolls sideways at 375                 | A wide table gets its own `overflow-x-auto` box or becomes a card list. The page itself never moves horizontally.                                                                                                                                                                              |
| 09  | Focus is visible everywhere                     | The global `:focus-visible` rule draws a 2px teal outline with a 2px offset on every interactive element. Restyle focus, never remove it. The keyboard order matches the visual order.                                                                                                         |
| 10  | Say what happened, then what to do              | Short, plain, direct. No "Oops", no exclamation marks, no emoji, and never a raw server message shown to a person.                                                                                                                                                                             |
| 11  | Public pages show cities, not people            | Anything reachable without an account (such as public tracking) shows status, cities and timestamps only. No names, phone numbers, addresses or fees.                                                                                                                                          |
| 12  | If you cannot remove it, you have not finished  | Delete anything that does not help the person do the task on that screen. No decorative numbers, no icon that repeats its label, no second border around a bordered thing.                                                                                                                     |

## 3. Look and feel

- **Palette.** Ink (near-black) and paper (warm off-white) with one lime accent and a teal for interactive things (links, focus). Success, warning, danger and info each have a text colour, a tint and a border. Do not add colours.
- **Fonts.** Three, all self-hosted through `next/font` in `app/layout.tsx`: Bricolage Grotesque (`font-display`) for display text and the page titles `h1` and `h2`, Instrument Sans (the default) for everything else, JetBrains Mono (`font-mono`) for identifiers and numbers. Do not add or swap fonts.
- **Icons.** Lucide only (`lucide-react`), thin line style: pass `strokeWidth={1.5}`. An icon never repeats the label next to it (rule 12).
- **Motion.** The travelling dot on the route line and the spinner are the only looping motion in the product. shadcn's one-shot open/close and menu animations are allowed; do not add looping animation. Transitions use `duration-fast`, `duration-base` or `duration-slow` with `ease-out-sd`. `prefers-reduced-motion` is already handled globally: do not add other animations.
- **Voice.** Sentence case. One plain sentence that says what happened, then one that says what to do. No jargon, blame, "Oops", exclamation marks or emoji. Examples: "Details saved." "That is not your current password." "We could not reach SwiftDrop. Check your connection and try again."
- **Light and dark.** Light is the default; dark is applied with `data-theme="dark"` on `<html>` (`lib/theme.ts`). Every semantic token has a dark value defined in `globals.css`, so a screen built from tokens works in both themes with no extra code. Two tokens do **not** flip: `ink` (always near-black) and `accent` (always lime). Therefore `bg-ink` is only for a surface that is dark in both themes, such as the brand panel of `AuthLayout`, and that surface opts in with `data-theme="dark"` so the semantic tokens inside it flip too. For every ordinary dark-on-light fill use `bg-primary text-on-primary`.

## 4. Tokens: the only values you may use

Token values live in `apps/web/src/app/globals.css`. The names below work as Tailwind utilities (`bg-*`, `text-*`, `border-*` and so on). `/design` shows them all in both themes.

### Colour

| Group                          | Classes                                                                                                                                       |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Page and surfaces              | `paper` (page background), `surface` (cards, inputs), `surface-2` (subtle fill, read-only, row hover), `surface-3` (pressed state, skeletons) |
| Lines                          | `line` (default hairline), `line-strong`, `line-control` (input borders)                                                                      |
| Text                           | `text` (default), `text-muted`, `text-faint`, `text-disabled`                                                                                 |
| Primary action                 | `primary`, `primary-hover`, `primary-active`, `on-primary`                                                                                    |
| Accent (lime)                  | `accent`, `accent-hover`, `accent-active`, `on-accent`                                                                                        |
| Interactive (teal)             | `interactive` (links, focus), `interactive-hover`, `interactive-tint`, `interactive-border`                                                   |
| Success, warning, danger, info | `success`, `warning`, `danger`, `info`, each with `-tint` and `-border`. Danger also has `danger-hover`, `danger-active` and `on-danger`.     |
| Parcel status                  | `status-<name>-fg`, `status-<name>-bg`, `status-<name>-bd` for `created`, `picked-up`, `in-transit`, `out`, `delivered`, `failed`             |
| Always-dark and overlay        | `ink`, `scrim` (behind a modal or drawer), `route-node` (the route line)                                                                      |

A parcel status is only ever one of six: the API names are `CREATED`, `PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, `FAILED`. The chip text is the name in capitals with spaces (`IN TRANSIT`, `OUT FOR DELIVERY`); prose uses sentence case ("In transit"). Never invent a seventh status in the UI. The six names are exported from `lib/tokens.ts`.

### Type

`display`, `h1`, `h2`, `h3`, `h4`, `body` (the default), `body-sm`, `label`, `caption`, `overline`, used as `text-<name>`. `font-display` goes with `display`, `h1` and `h2`. `overline` is uppercase (`text-overline uppercase`). `text-button` and `text-body-lg` exist for the text inside controls and are already used by the kit, so a screen normally does not need them. Use `tabular` for columns of numbers.

`cn()` in `lib/utils.ts` registers these names with tailwind-merge, so a class list such as `text-body-sm text-text` keeps both the size and the colour. Keep new type names in that list when you add one.

### Space, shape, depth, motion, breakpoints

- **Spacing** is a 4px base. For padding, margin and gap use only these steps: `1`, `2`, `3`, `4`, `5`, `6`, `8`, `10`, `12`, `16`, `20`, `24` (so `p-4` is 16px). The shell already puts 48px (`gap-12`) between sections.
- **Radius:** `rounded-chip` (6px, chips and badges), `rounded-control` (8px, buttons and inputs), `rounded-card` (12px, cards and alerts), `rounded-splash` (the splash screen only), `rounded-full` (avatars). No other radius.
- **Shadow:** `shadow-popover` and `shadow-modal`, nothing else (rule 02). Foundation primitives may also use shadcn's semantic `shadow-md`/`shadow-lg` aliases, which resolve to those two tokens.
- **Motion:** `duration-fast` (120ms), `duration-base` (160ms), `duration-slow` (200ms), `ease-out-sd`.
- **Breakpoints (mobile first):** `sm` 375px, `md` 768px, `lg` 1024px, `xl` 1440px. The content width is `max-w-content`.
- **Foundation geometry:** generated primitives may use Radix-required positioning variables and calculated modal widths (for example `translate-y-[calc(...)]` or `max-w-[calc(...)]`). These are functional geometry, not screen styling; screens must not copy them.

### Examples

| Wrong                                 | Right                                          | Why                                                  |
| ------------------------------------- | ---------------------------------------------- | ---------------------------------------------------- |
| `bg-[#ffffff] text-[#5c6068]`         | `bg-surface text-text-muted`                   | Raw values break dark mode and the design (rule 06). |
| `bg-red-500`, `text-sm`, `rounded-lg` | `bg-danger`, `text-body-sm`, `rounded-control` | The Tailwind defaults are removed and do nothing.    |
| `bg-ink text-paper` on a badge        | `bg-primary text-on-primary`                   | `ink` does not flip in dark mode; the text vanished. |
| `shadow-md` on a card                 | `border border-line`                           | Hairlines before shadows (rule 02).                  |
| `p-[13px]`, `gap-7`                   | `p-3`, `gap-8`                                 | Off-scale spacing.                                   |
| `<h1>` written by hand                | `PageHeader`                                   | It is the one h1 of the page.                        |
| `style={{ color: '#c23b2e' }}`        | `text-danger`                                  | No inline looks.                                     |

## 5. Project structure and naming

```
apps/web/src/
  app/
    (auth)/         public auth pages: login, register, reset password, signed-out
    (public)/       pages anyone can open (public tracking): PublicLayout, no login
    (app)/          EVERY signed-in screen goes here
      layout.tsx    session check and the role's shell. Do not edit.
      dashboard/    profile/    admin/users/    (examples)
      <route>/page.tsx          <- a new screen
    design/         the style guide page
    globals.css     design tokens and the shadcn token bridge. Protected.
  components/
    ui/             shadcn primitives and SwiftDrop wrappers (section 7)
    layout/         PageHeader, Section, RequireRole, SystemPage, PublicLayout, the shells
    auth/           login, register and reset screens
    profile/        a feature folder and a worked example
    notification-logs/  the ADMIN notification-log screen
    <feature>/      your components
  lib/
    api.ts          api(), apiWithMeta(), ApiError, onSessionExpired()
    auth-api.ts     example of a feature's API file
    nav.ts          the sidebar entries and landing page for every role
    session.tsx     useSession()
    utils.ts        cn(), configured for the SwiftDrop type scale
    initials.ts     server-safe initials helper used by Avatar
    use-theme-preference.ts  shared theme state
    use-form.ts, validation.ts, password.ts, use-countdown.ts, format.ts, cn.ts, tokens.ts, roles.ts, theme.ts
    notification-logs-api.ts  the ADMIN notification-log calls and types
    <feature>-api.ts   your API calls and types
apps/web-e2e/src/   Playwright tests; support/mock-api.ts and support/sign-in.ts
```

`apps/web/components.json` configures shadcn/ui. It is not under `src/`.

**Naming (from the design system):**

- Files are kebab-case (`orders-view.tsx`, `parcels-api.ts`). Components are PascalCase with the noun first (`DataTable`, not `TableData`).
- A variant is a prop, never a new name: `<Button variant="danger">`, never `DangerButton`.
- Booleans read as `is` or `has` (`isLoading`, `isDisabled`, `hasError`). Handlers read as `on` (`onSelect`, `onDismiss`).
- Token names are kebab-case, category first, and never contain a value (`--color-accent`, never `--color-lime`).
- A `page.tsx` is a small server file: it sets the `metadata` title and renders your view. The view is a client component (`'use client'`) because it uses state, forms or the session.
- Import with relative paths.

## 6. Layout templates, the shell and roles

There are exactly four layout templates. Do not invent a fifth.

| Template       | For                                                        | State                                                                                                           |
| -------------- | ---------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `AuthLayout`   | login, register, password reset                            | exists (`components/layout/auth-layout.tsx`)                                                                    |
| `AppShell`     | signed-in customer, merchant, dispatcher and admin screens | exists, picked for you by `RoleShell`                                                                           |
| `CourierShell` | signed-in courier screens (mobile first, bottom tabs)      | exists, picked for you by `RoleShell`                                                                           |
| `PublicLayout` | pages anyone can open, such as public tracking             | exists (`components/layout/public-layout.tsx`), used by the `(public)` route group. Obey rule 11 on such pages. |

**What the `(app)` group does for you.** `app/(app)/layout.tsx` checks the login cookie (a splash, then a redirect to `/login` that returns afterwards), picks the shell for the signed-in role, and shows the 403 and 404 pages inside it. A sidebar entry whose page is not built shows the 404. When the 15-minute access token expires, `api()` renews it and retries. If the session cannot be renewed while the person is on a page, the page stays and the "Your session ended" dialog opens over it; its one button, Log in, goes to `/login` and back to the same address, query string included.

**A public page** goes in `app/(public)/<route>/page.tsx`. The group gives it `PublicLayout` (logo, theme switch, Log in, one reading column) and no login check, so a signed-out visitor and a signed-in one both see it. Everything on it is public: show status, cities and times, never names, phone numbers, addresses or fees (rule 11). `/track` is the worked example (API 4.6): `PageHeader`, then a form, `WaybillStub isPublic` and `Timeline`.

**A new screen** is two files:

```tsx
// apps/web/src/app/(app)/orders/page.tsx
import type { Metadata } from 'next';
import { OrdersView } from '../../../components/orders/orders-view';

export const metadata: Metadata = { title: 'Orders — SwiftDrop' };

export default function OrdersPage() {
  return <OrdersView />;
}
```

```tsx
// apps/web/src/components/orders/orders-view.tsx
'use client';

import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';

export function OrdersView() {
  return (
    <RequireRole roles={['MERCHANT']} audience="merchants">
      <PageHeader
        title="Orders"
        description="Parcels created through your API key."
        actions={
          <ButtonLink href="/api-keys" variant="secondary">
            API keys
          </ButtonLink>
        }
      />
      <Section title="Recent orders">{/* content */}</Section>
    </RequireRole>
  );
}
```

`PageHeader` takes `title`, one sentence of `description` and at most two `actions`, only one of them primary. `Section` takes a `title`, an optional `description` and `actions`. `PageHeader` renders the page's one `h1` and each `Section` an `h2`: never write another `h1`.

**Roles and navigation.** The roles are `CUSTOMER`, `MERCHANT`, `COURIER`, `DISPATCHER`, `ADMIN`. An `ADMIN` also passes every `DISPATCHER` check (`lib/roles.ts`). The sidebar entries for each role live in `lib/nav.ts`:

| Role       | Entries (route)                                                                                                                   |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------- |
| CUSTOMER   | Dashboard `/dashboard`, New delivery `/new`, My parcels `/parcels`, Profile `/profile`                                            |
| MERCHANT   | Dashboard, Orders `/orders`, API keys `/api-keys`, Webhook `/webhook`, Profile                                                    |
| COURIER    | Today `/dashboard`, History `/history`, Profile (bottom tabs)                                                                     |
| DISPATCHER | Unassigned `/unassigned`, Workload `/workload`, All parcels `/parcels`, Reports `/reports`                                        |
| ADMIN      | the dispatcher's entries, plus Users `/admin/users`, Merchants `/admin/merchants`, Zones `/admin/zones`, Pricing `/admin/pricing` |

- If your route is missing, add one entry with a Lucide icon. Do not reorder or remove other people's entries.
- `/dashboard` is shared by every role and `/parcels` by customers and dispatchers. A route has one `page.tsx`, so its view must choose by role with `useSession()` (`session.status === 'authenticated'`, then `session.user.role`). Agree with the other owner who writes that file.
- After login every role lands on `/dashboard` until you change that role's entry in `roleHome` in `lib/nav.ts` when you build its first real screen.
- **Hiding a link never protects a page.** Wrap a role-limited page in `RequireRole`; anyone else sees the 403 page inside the shell. That is the no-permission state. The API checks the role again.
- Put a signed-in screen outside `app/(app)/` and it gets no shell and no login check.

## 7. The component kit

Import from the file with a relative path. Each file starts with a comment that explains it. Do not copy a component into your feature folder; use it, or ask for a change (section 14). The shadcn semantic names (`background`, `card`, `primary`, `muted`, `border`, `ring` and so on) are bridged to SwiftDrop tokens in `globals.css`; screens still use the SwiftDrop token classes in section 4.

**Layout (`components/layout/`)**

| Component      | Use it for                      | Notes                                                                                                                                                                                          |
| -------------- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PageHeader`   | the top of every page           | `title`, `description`, `actions` (max two, one primary)                                                                                                                                       |
| `Section`      | a heading and its content       | `title`, `description`, `actions`. Sections space themselves.                                                                                                                                  |
| `RequireRole`  | limiting a page to some roles   | `roles`, `audience` (a plural noun phrase such as "merchants"). Shows the 403 page inside the shell.                                                                                           |
| `SystemPage`   | 404, 403, 500 and offline pages | `label`, `title`, children (what to do), `actions`, `reference` (only for a fault on our side), `headingLevel` (leave it at 1; a style-guide demo passes 2). One sentence about what happened. |
| `AuthLayout`   | public auth pages only          | `panel`, `width`, children. Used with `AuthHeading`.                                                                                                                                           |
| `PublicLayout` | pages anyone can open           | `width` (`narrow` reading column, or `wide`), children. Wrapped around the `(public)` route group for you; you rarely use it directly.                                                         |

`AppShell`, `CourierShell`, `RoleShell`, `NavList`, `Drawer` and `UserMenu` are internal to the shell. Do not import them in a screen.

**Controls and feedback (`components/ui/`)**

| Component                                           | Use it for                                     | Key props and rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| --------------------------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Button`                                            | an action                                      | `variant`: `primary`, `secondary`, `accent` (marketing only), `ghost`, `danger`, `link`. `size`: `sm`, `md`, `lg`. `isLoading` (spinner, blocks a second click; keep the label meaningful, such as "Saving"), `isDisabled`, `isFullWidth`, `iconLeading`, `iconTrailing`.                                                                                                                                                                                                                                   |
| `ButtonLink`                                        | navigation that looks like a button            | Same looks as `Button`; use it when the action is a link. `buttonClasses()` gives the classes for other link-like elements.                                                                                                                                                                                                                                                                                                                                                                                 |
| `TextField`                                         | any text input                                 | Needs `id` and `label`. Optional `helper`, `error`, `success`, `isRequired`, `controlSize` (`lg` for touch screens), `isMono` (IDs, amounts), `prefix` and `suffix` (such as `+94` or `LKR`). An error replaces the helper text. For a date or a time use `type="date"`, `"time"` or `"datetime-local"`: the browser's own picker in the same box. The value is local (`2026-11-05`); send the API an ISO time in UTC.                                                                                      |
| `PasswordField`                                     | passwords                                      | `autoComplete` is `current-password` or `new-password`. `showChecklist` shows the strength bars and rules for a new password.                                                                                                                                                                                                                                                                                                                                                                               |
| `Field`, `controlClasses`                           | a control the kit does not have yet            | Wraps your control so the label, messages and look match. Use it only until the kit gets that control, and tell M2.                                                                                                                                                                                                                                                                                                                                                                                         |
| `Alert`                                             | a message inside the page                      | `tone`: `info`, `success`, `warning`, `danger`; `title`, children, `actions`, `icon`. Danger interrupts screen readers; the other tones wait.                                                                                                                                                                                                                                                                                                                                                               |
| `Banner`                                            | a page-level notice under the top bar          | `tone`: `info`, `warning`, `danger`; `title`, children, `action`, `onDismiss`. Show one at a time. Only an info banner should be dismissible.                                                                                                                                                                                                                                                                                                                                                               |
| `ErrorSummary`                                      | the list of problems after a failed submit     | `issues` (each has `fieldId`, `label`, `message`), `requestId` (5xx only), `focusOnMount`. It does not replace the message beside each field.                                                                                                                                                                                                                                                                                                                                                               |
| `Spinner`                                           | loading                                        | `size` 16 or 20, `tone`, `label`. For a whole page use `tone="standalone"` with a `label`.                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `Skeleton`, `SkeletonText`, `SkeletonGroup`         | the loading state of a list, table or card     | Put the blocks in a `SkeletonGroup` with a `label` such as "Loading parcels". `Skeleton` is one block sized with token classes (`h-5 w-1/3`); `SkeletonText` is `lines` of text with a shorter last line. They are still on purpose (no pulse): only the spinner and the route line move. Use `Spinner` for a whole page or a button.                                                                                                                                                                       |
| `EmptyState`                                        | a list, table or section with nothing in it    | `title` ("No parcels yet"), one sentence of children that says what to do, one `action` (a primary `Button` or `ButtonLink`), optional Lucide `icon`. Its title is an `h3`, so use it inside a `Section`. Without an action when nothing can fix it, such as "No results" after a filter.                                                                                                                                                                                                                   |
| `Checkbox`, `CheckboxGroup`                         | one yes or no; several related choices         | `Checkbox`: `id`, `label`, `description`, `helper`, `error`, `isRequired`, `isIndeterminate` (a "select all" with only some ticked), and the input's own props. `CheckboxGroup`: `name`, `legend`, `options` (`value`, `label`, `description`, `isDisabled`), `values`, `onChange(values)`, `helper`, `error`, `isRequired`, `isDisabled`. The whole row is a 44px target.                                                                                                                                  |
| `RadioGroup`                                        | exactly one choice from a short list           | `name`, `legend`, `options` as above, `value` (`''` while nothing is chosen), `onChange(value)`, `helper`, `error`, `isRequired`, `orientation` (`horizontal` fits two or three short options from 768px). For a long list use `Select`.                                                                                                                                                                                                                                                                    |
| `FileField`                                         | picking one file, such as a proof photo        | `id`, `label`, `helper`, `error`, `accept`, `capture="environment"` (opens a phone's camera), `buttonLabel`, `emptyLabel`, `controlSize` (`lg` for courier screens), `onFileChange(file or null)`. Check the type and size yourself and pass your own sentence as `error`; `accept` is only a hint.                                                                                                                                                                                                         |
| `IconButton`                                        | an action with no visible text                 | `icon`, `label` (required: the tooltip and the accessible name, such as "Actions for SDK7M2Q9XP4"), `variant` (`ghost`, `secondary`, `danger`), `size` (`sm` 36px, `md` 44px, `lg` 48px), `isLoading`, `isDisabled`. Leave 8px between neighbours. Use a `Button` with a word when there is room.                                                                                                                                                                                                           |
| `KeyValueList`                                      | facts about one thing                          | `items` (`label`, `value`, `isMono`), `layout` (`stacked`, or `inline` from 768px). An empty value shows a dash that screen readers read as "Not provided".                                                                                                                                                                                                                                                                                                                                                 |
| `WaybillStub`                                       | a parcel as a paper waybill                    | `trackingNumber`, `status`, `origin`, `destination` (cities), `footRows` (`KeyValueList` items; mark names, phones, addresses and fees `isPrivate`), `isPublic` (drops every private row and says what is shown), `isCompact` (one row, for a list), `isLoading`.                                                                                                                                                                                                                                           |
| `Textarea`                                          | several lines of text                          | Same props as `TextField` (`id`, `label`, `helper`, `error`, `success`, `isRequired`, `controlSize`) plus `rows` (default 4) and `showCount` (shows "12/200"; needs `maxLength` and a controlled `value`).                                                                                                                                                                                                                                                                                                  |
| `Select`                                            | one choice from a list                         | `id`, `label`, `options` (`value`, `label`, `isDisabled`), `placeholder` (an unchosen first entry such as "Choose a zone"), and the same messages as `TextField`. It is the browser's own select. For a zone list, load `GET /zones` and map it to `options`.                                                                                                                                                                                                                                               |
| `Modal`                                             | a decision or a small input in a dialog        | `isOpen`, `onClose`, `title`, `description`, children, `actions` (one primary or danger button and a secondary Cancel), `isDismissible` (false for a must-answer dialog), `size`. Native dialog: focus is trapped, Escape closes, focus returns. Mark the element that should receive focus first with `data-autofocus` (the field, or the safe button of a destructive confirmation).                                                                                                                      |
| `StatusChip`                                        | any status in a list or header                 | `status`: the six parcel statuses, the merchant states (`PENDING`, `APPROVED`, `REJECTED`, `SUSPENDED`) and `ACTIVE`, `DEACTIVATED`. A shape and the name in capitals, never colour alone. `isCompact` keeps only the shape, with the name as a tooltip and for screen readers.                                                                                                                                                                                                                             |
| `DataTable`, `DataTableCardList`, `TablePagination` | lists of rows                                  | `DataTable` takes `caption`, `columns` (`key`, `header`, `cell`, `isNumeric`, `isMono`, `isSortable`, `isHeaderHidden` for an actions column), `rows`, `getRowId`, `isLoading`, `empty` (an `EmptyState`), and `sort` with `onSortChange` (your API sorts; the table only shows the arrow). From 768px it is a table in its own scrolling box; below that the same columns become cards. `TablePagination` takes `page` (from 1), `totalPages`, `totalItems`, `onPageChange` and hides itself for one page. |
| `Toast`, `useToast()`                               | a short confirmation with no place on the page | `useToast().show({ title, description, tone, duration })`: tone defaults to success, `duration` to 5000 ms (0 keeps it until dismissed). Hover or focus pauses it; three show at most. The provider is mounted in `app/layout.tsx`, so call it from any client component. Use an `Alert` when the message must stay on the page.                                                                                                                                                                            |
| `Timeline`                                          | what happened to a parcel or an order          | `label`, `items` (`id`, `title`, `at` ISO time, `detail`, `isCurrent`, `isUpcoming`). Times show in Sri Lanka time (`formatDateTime` in `lib/format.ts`). On a public page `detail` is a city, never a person (rule 11).                                                                                                                                                                                                                                                                                    |
| `RouteLine`                                         | the brand motif                                | Decorative: put the meaning in nearby text. `state`: `idle`, `travelling`, `complete`.                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `RoleBadge`, `Avatar`                               | showing a role or initials                     | Decorative: always write the name or role next to them.                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `Logo`, `LogoMark`, `ThemeSwitch`                   | brand and theme                                | Already in the shells. Do not place them in a screen.                                                                                                                                                                                                                                                                                                                                                                                                                                                       |

**shadcn primitives (`components/ui/`)**

| Component                                                                         | Use it for                              | Notes                                                                                                                                                                                                                                                                                                           |
| --------------------------------------------------------------------------------- | --------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter` | a grouped surface                       | Hairline border, no shadow.                                                                                                                                                                                                                                                                                     |
| `Badge`                                                                           | a short label or status                 | `variant`: `default`, `secondary`, `destructive`, `outline`. A status still carries its name and shape (rule 04).                                                                                                                                                                                               |
| `Input`, `TextareaControl`, `Label`                                               | plain form controls                     | `Textarea` is the labelled SwiftDrop wrapper; `TextareaControl` is the bare primitive. Prefer `TextField` when you need a label, helper, error and success message.                                                                                                                                             |
| `SelectMenu`, `CheckboxControl`, `RadioGroupControl`, `RadioGroupItem`            | bare primitives for custom compositions | `Select` remains the browser's native labelled select; `SelectMenu` is the shadcn/Radix listbox. `Checkbox`/`CheckboxGroup` and `RadioGroup` above are the labelled SwiftDrop form controls; `CheckboxControl`/`RadioGroupControl` are the bare Radix primitives. Use `Label` with `htmlFor` for every control. |
| `Dialog`                                                                          | a confirmation or short form            | Radix traps focus and closes on Escape. Use `DialogTitle` and `DialogDescription`. `Modal` remains the native decision dialog.                                                                                                                                                                                  |
| `Sheet`                                                                           | a drawer or side panel                  | The mobile navigation drawer uses it.                                                                                                                                                                                                                                                                           |
| `DropdownMenu`                                                                    | a menu of actions                       | Radix handles arrow keys, Home/End, Escape and typeahead.                                                                                                                                                                                                                                                       |
| `Tooltip`                                                                         | a short hint on an icon-only control    | `NavList` already wraps rail links in `TooltipProvider`; wrap a new standalone tooltip in its own provider.                                                                                                                                                                                                     |
| `Separator`                                                                       | a hairline between groups               | Use it instead of a one-off border.                                                                                                                                                                                                                                                                             |
| `Table`                                                                           | tabular data                            | Put it in a container that can scroll sideways on small screens, or use a card list.                                                                                                                                                                                                                            |
| `Pagination`                                                                      | paging through a list                   | Uses the same button looks as the kit.                                                                                                                                                                                                                                                                          |
| `Empty`                                                                           | the primitive behind `EmptyState`       | Prefer `EmptyState` unless you need a custom composition.                                                                                                                                                                                                                                                       |

**Feature pieces you may reuse**

- `components/auth/form-alerts.tsx`: `ServerFaultAlert` (a fault on our side or the network, with a Reference line for 5xx only) and `RateLimitedAlert` (a 429, with the remaining seconds). Use these instead of writing your own.
- `components/auth/session-gates.tsx`: `SignedInOnly` and `SignedOutOnly`. The `(app)` and `(auth)` groups already use them.
- `components/auth/session-expired-dialog.tsx`: `SessionExpiredDialog`, the must-answer "Your session ended" dialog. `SignedInOnly` mounts it when `useSession().isExpired` is true; a screen never renders it.
- `components/profile/`: `ProfileForm` and `PasswordForm` are the worked examples of a complete form screen.

**Hooks and helpers (`lib/`)**

- `useSession()` returns `{ session, signIn, signOut, setUser, recheck }`. `session.status` is `loading`, `authenticated`, `unauthenticated` or `unavailable`. When authenticated, `session.user` has `id`, `email`, `fullName`, `phone`, `role`, and sometimes `authProvider` and `createdAt`.
- `useForm`, `validation.ts`, `use-countdown.ts` (for "try again in 30 seconds") and `cn()` (joins class names).
- `lib/tokens.ts` exports typed names (`Role`, `ParcelStatus`, the token lists). Keep it in step with `globals.css` when a token is added.

## 8. What does not exist yet

These are not built. They are designed and planned and will be added when a screen needs one. **Do not build your own version.** Until the foundation owner adds one, leave a `// TODO(design-foundation): needs <name>` comment and a plain placeholder, and tell the person you are working for.

| Not built yet                                        | For                                                                                                                                 |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `Switch`, `Tabs`, `Stepper`, `Popover`, `Breadcrumb` | designed in the design system's library but not built; ask for one when a screen needs it (the delivery wizard will want `Stepper`) |

When the task cannot wait, say so in the pull request instead of styling a one-off, as the design system asks. A new component must follow section 14, use only tokens, appear on `/design` with its states, and have tests.

## 9. Forms

Copy `components/profile/profile-form.tsx` (an edit form) or `components/profile/password-form.tsx`. The pattern:

```tsx
type Values = { recipient: string };

// Define the rules OUTSIDE the component so they stay the same object.
const rules: FormRules<Values> = {
  recipient: (v) => validateFullName(v.recipient),
};

const form = useForm<Values>({ recipient: '' }, rules);

<form noValidate onSubmit={submit}>
  <TextField
    id="parcel-recipient"
    label="Recipient name"
    isRequired
    value={form.values.recipient}
    error={form.errors.recipient}
    onChange={(e) => form.setValue('recipient', e.target.value)}
    onBlur={() => form.blur('recipient')}
  />
  <Button type="submit" isLoading={isLoading}>
    Create parcel
  </Button>
</form>;
```

- **Timing.** Nothing is validated until a field has been left once. After a failed submit, fields re-validate on every change so an error clears the moment it is fixed. `useForm` does this.
- **On submit** call `form.validate()`. If it finds a problem, move focus to the first bad field and stop. Do not send the request.
- **A 422 from the API** carries `error.issues[].field`. Map each field to our own wording with `serverFieldMessage()` and call `form.setServerErrors(...)`. Add a message for a new field to `serverMessages` in `lib/validation.ts`. Never print the server's text.
- **Keep what was typed** after any failure. Show `isLoading` while saving. After success, say so in plain words (`Alert tone="success"`) or move on.
- **Edit forms send only what changed**, and the Save button stays off until something changes (see the profile form).
- **A phone number** is E.164 for the API (`+94771234567`). A Sri Lankan number typed after a fixed `+94` prefix goes through `toSriLankanE164()`. A password follows the same rules everywhere: at least 8 characters, at most 72 UTF-8 bytes, with an upper case letter, a lower case letter and a digit (`lib/password.ts`).
- Every field has a real `label`, and an `id` that is unique on the page.

## 10. Talking to the API

- Browser code calls `/api/v1/...` only through `api<T>(path, options)` or `apiWithMeta<T>(path, options)` in `lib/api.ts`. Paths are relative to `/api/v1`. `api()` returns the `data` of the success envelope (and `undefined` for a 204). `apiWithMeta()` also returns `meta`, which carries `pagination` (`page`, `pageSize`, `totalItems`, `totalPages`) for list endpoints.
- Put a feature's calls and types in `lib/<feature>-api.ts`, with the field names exactly as `docs/API_Documentation_v1.1.md` writes them. If the API does not match what the screen needs, fix the API document and the contract in the same change; do not work around it in the screen.
- The session lives in HttpOnly cookies. You never read or store a token.
- PDF downloads use `apiBlob()` from the same API module, including session renewal and common errors. Creation requests may pass `headers: { 'Idempotency-Key': key }`; preserve that key on retries. `PRICE_CHANGED` exposes a validated numeric `currentFee` for explicit reconfirmation, never server message text.
- If an access token has expired, `api()` renews the session once and retries. If that fails, the session-ended dialog opens over the page (someone who never had a session goes straight to the login page). A `401 INVALID_CREDENTIALS` is a wrong password, not an expired session, and is left alone.
- A failure throws `ApiError`. Decide by `error.code`, never by message text:

| `error.code`                                                                                                                            | It means                                 | What a screen does                                                                         |
| --------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- | ------------------------------------------------------------------------------------------ |
| `VALIDATION_FAILED`                                                                                                                     | 422: a field is invalid                  | Our own wording beside each field (`error.issues`). Keep the input.                        |
| `UNAUTHENTICATED`                                                                                                                       | 401: not signed in, or the session ended | Nothing: the shell shows the session-ended dialog, or sends a cold visit to login.         |
| `FORBIDDEN`                                                                                                                             | 403: this role may not do this           | The no-permission state. Do not retry.                                                     |
| `ACCOUNT_DEACTIVATED`                                                                                                                   | the account was disabled                 | Call `recheck()` from `useSession()`; the shell signs the person out.                      |
| `NOT_FOUND`                                                                                                                             | 404: missing, or not theirs              | A "not found" state.                                                                       |
| `RATE_LIMITED`                                                                                                                          | 429: too many requests                   | `RateLimitedAlert`, and wait `error.retryAfter` seconds.                                   |
| `INTERNAL`, `NETWORK`                                                                                                                   | our fault, or no connection              | `ServerFaultAlert`. Show `error.requestId` as "Reference" for these only, never for a 4xx. |
| `INVALID_CREDENTIALS`, `ACCOUNT_LOCKED`, `EMAIL_IN_USE`, `PASSWORD_NOT_SET`, `RESET_TOKEN_INVALID`                                      | account flows                            | Used by the login, register, reset and profile screens.                                    |
| `ZONE_NOT_FOUND`, `ROUTE_NOT_SERVICEABLE`, `WEIGHT_NOT_PRICED`, `ZONE_CODE_EXISTS`, `RULE_EXISTS`, `BAND_OVERLAP`, `TRACKING_NOT_FOUND` | pricing and tracking                     | Our own sentence for each, beside the field where one applies, or a "not found" state.     |

- An API code that is not mapped yet falls back to its HTTP status. Add it to `ApiErrorCode` and `codeFromApi` in `lib/api.ts`, in a small change, and tell M2.
- Never show `error.message`. It is written for developers and may contain internal detail.

## 11. Screen states and loading data

Every screen designs four states (rule 07):

| State         | What to show                                                                                                                                                           |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Loading       | `DataTable isLoading` for a list, or `Skeleton` blocks in a `SkeletonGroup` where a card will appear. `Spinner` (`tone="standalone"` with a `label`) for a whole page. |
| Empty         | `EmptyState`: what is missing and the one action that fixes it: "No parcels yet. Create your first delivery."                                                          |
| Error         | What went wrong and "try again", in an `Alert tone="danger"` or `ServerFaultAlert`. Keep the person's input.                                                           |
| No permission | `RequireRole` (the 403 page inside the shell).                                                                                                                         |

Confirm a successful action in plain words ("Details saved."), in an `Alert tone="success"` or, for a quick confirmation after a dialog or a row action, a `Toast`.

There is no data-fetching library, by decision. Fetch in a client view with a three-case state and an `AbortController`, as `components/profile` and `lib/session.tsx` do:

```tsx
const [state, setState] = useState<
  | { status: 'loading' }
  | { status: 'ready'; parcels: Parcel[] }
  | { status: 'error'; error: ApiError }
>({ status: 'loading' });

useEffect(() => {
  const controller = new AbortController();
  listParcels(1, controller.signal)
    .then(({ data }) => setState({ status: 'ready', parcels: data }))
    .catch((error: unknown) => {
      if (!controller.signal.aborted)
        setState({ status: 'error', error: error as ApiError });
    });
  return () => controller.abort();
}, []);
```

## 12. Responsive, accessibility and theming

- **375px is the smallest width.** Nothing scrolls sideways and nothing is clipped (rule 08). Touch targets are at least 44px, and 48px on courier screens (use `controlSize="lg"` and `size="lg"` buttons there). The kit does the 44px for you: `sm` and `md` buttons, fields, selects, icon buttons and the shell's own buttons are 32 to 40px with a mouse and grow to 44px on a touch screen (`pointer-coarse:`); nothing to add in a screen. A checkbox or radio is its whole 44px row. Check the screen at about 360–375px and at 1280–1440px.
- **Dark mode.** Switch to dark with the toggle in the top bar and check every surface, border and status colour. Semantic tokens flip by themselves; see section 3 for `ink` and `accent`.
- **Keyboard.** Tab through the screen: visible focus on everything (rule 09), a sensible order, Escape closes overlays, and focus returns to the control that opened them. A native `<dialog>` (as the navigation drawer uses) gives you the trap and Escape for free.
- **Semantics.** Every control has a real label. Headings go in order. Use a button for an action and a link for navigation. Never rely on colour alone to carry meaning. ESLint runs `jsx-a11y`: do not silence its warnings.
- **Lime** is never a text colour and never a state (rules 04 and 05).

## 13. Testing

Tests use Playwright with a mocked API, so a screen is tested without a backend.

```ts
// apps/web-e2e/src/orders.spec.ts
import { test, expect } from '@playwright/test';
import { fail, ok } from './support/mock-api';
import { signInAs } from './support/sign-in';

test('a customer sees their parcels', async ({ page }) => {
  await signInAs(page, 'CUSTOMER', {
    // key = METHOD + path, without /api/v1 and without the query string
    'GET /parcels': ok([
      { id: '1', trackingNumber: 'SDK7M2Q9XP4', status: 'CREATED' },
    ]),
  });
  await page.goto('/parcels');
  await expect(page.getByText('SDK7M2Q9XP4')).toBeVisible();
});
```

- `signInAs(page, role, extraHandlers?, userOverrides?)` signs the page in as any of the five roles. `ok(data)` and `fail(status, 'CODE')` build replies, and a handler may be a function that reads the request body. For a list with pagination, return a reply object whose `body.meta` has `pagination`.
- Test the empty, error, no-permission and success states, not only the happy path. Copy from `profile.spec.ts` (forms: validation, a 422, a 429, a server fault) and `app-shell.spec.ts` (role guards).
- Run:

```bash
cd apps/web-e2e && pnpm exec playwright test --project=chromium
pnpm exec nx run-many -t lint typecheck --projects=@org/web,@org/web-e2e
```

If a run hangs or many tests fail at once, a leftover dev server is the usual cause: `pkill -f "next dev"` and run again.

## 14. Protected files and changing the foundation

Do not edit these as part of a normal screen. They are shared by every screen, and a change to them changes the design language for everyone.

- `apps/web/src/app/globals.css`, `apps/web/components.json`, and the token lists in `apps/web/src/lib/tokens.ts`
- `apps/web/src/components/ui/*`, `apps/web/src/lib/utils.ts`, `apps/web/src/lib/initials.ts`, `apps/web/src/lib/use-theme-preference.ts`, and `apps/web/src/components/layout/*`
- `apps/web/src/app/(app)/layout.tsx` and `apps/web/src/app/layout.tsx`
- `apps/web/src/lib/api.ts`, `lib/session.tsx`, `lib/use-form.ts`, `lib/validation.ts` (adding a shared validator or a field message is fine), `lib/theme.ts`
- `apps/web-e2e/src/support/*`
- `apps/web/eslint.config.mjs` and `apps/web/eslint-rules/*` (the lint rules that enforce this file)
- the structure of `apps/web/src/lib/nav.ts` (adding your own entry is fine)

If a screen needs a change to one of them, or a component that does not exist, ask the foundation owner (M2) first. An approved change must:

1. use tokens only, and never change an existing token's name;
2. keep the component API in the style of the kit (variants and sizes as props, `is`/`has` booleans, `on` handlers);
3. add or update its section on the `/design` page, with every state;
4. add Playwright tests, and keep the dark theme working;
5. update this file, and `AGENTS.md` when a rule others must follow changes;
6. say in the pull request what changed and why.

## 15. Before you finish

Run through this list and fix whatever fails. In a pull request, name any item you could not satisfy.

- [ ] The screen lives under `app/(app)/<route>/`, starts with `PageHeader`, and uses `Section` blocks.
- [ ] Every control, message and layout piece comes from the kit. Nothing is copied or restyled.
- [ ] There is one primary action, and no shadow except on a popover or modal.
- [ ] Only token classes appear: no hex, no arbitrary values, no default Tailwind classes, no off-scale spacing, no inline `style`.
- [ ] Loading, empty, error and no-permission states exist, and a successful action is confirmed.
- [ ] Identifiers and amounts are in `font-mono`; a status has a label and a shape.
- [ ] A form follows section 9, keeps what was typed after a failure, and never shows server text.
- [ ] API calls go through `api()` or `apiWithMeta()` in `lib/<feature>-api.ts`, with fields named as in the API document. A changed API path, field or error code is also changed in the API document and the OpenAPI contract.
- [ ] Nothing scrolls sideways at 375px, and touch targets are at least 44px (48px for couriers).
- [ ] The screen looks right in dark mode, and works with the keyboard alone.
- [ ] Copy is short and plain, says what happened and what to do, and has no "Oops", exclamation mark or emoji.
- [ ] Public pages show cities, statuses and timestamps only.
- [ ] Playwright tests cover the states above, and `lint`, `typecheck` and the whole suite pass.
- [ ] You added no `eslint-disable` comment to get lint to pass. A real exception has a reason written after `--`.
- [ ] No new dependency beyond the approved shadcn/ui foundation was added, and no new font, icon set or colour. No protected file was edited (section 14).

## 16. What lint checks for you

This file is enforced in code. `pnpm exec nx lint @org/web` runs ESLint rules from `apps/web/eslint-rules/`, and CI ("Frontend checks") runs the same lint. A violation fails the check with a message that says what to use instead and points back to this file.

| What it catches                                                                                                                                                                                                                          | Rule                              | Example, and what to use                                |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ------------------------------------------------------- |
| Colours that are not tokens: a hex, `rgb()` or `hsl()` in a class list, a colour attribute or a style object                                                                                                                             | `swiftdrop/no-raw-colors`         | `bg-[#fff]`, `fill="#4285F4"` -> `bg-surface`           |
| Class lists that leave the tokens: arbitrary values, default Tailwind colours, text sizes, radii and shadows, off-scale spacing, `bg-ink` and `text-paper`, the `2xl:` breakpoint. (`grid-cols-[...]` and `[var(--token)]` are allowed.) | `swiftdrop/tailwind-tokens`       | `p-7` -> `p-6` or `p-8`; `rounded-lg` -> `rounded-card` |
| An inline `style` prop                                                                                                                                                                                                                   | `swiftdrop/no-inline-style`       | `style={{ ... }}` -> token classes                      |
| UI libraries, other icon sets, CSS-in-JS, state-management and data-fetching libraries, extra fonts. `radix-ui` is allowed only inside `components/ui`.                                                                                  | `no-restricted-imports`           | `react-icons` -> `lucide-react`                         |
| A direct network call                                                                                                                                                                                                                    | `no-restricted-globals` (`fetch`) | `fetch(...)` -> `api()`                                 |
| Showing `error.message` to a person                                                                                                                                                                                                      | `no-restricted-syntax`            | `{error.message}` -> our own sentence, by `error.code`  |
| A hand-written `<h1>` in a screen                                                                                                                                                                                                        | `no-restricted-syntax`            | `<h1>` -> `PageHeader`                                  |

**Where it applies.** Screens are strict: everything under `apps/web/src` except the foundation. The foundation files (`components/ui`, `components/layout`, `app/design`, `app/layout.tsx`, `lib/` and the brand panel) are exempt from the class rules because they define the tokens, but they still must not contain raw colours. Generated primitives may use the shadcn semantic aliases and Radix-required functional geometry described in section 4; screens must not. The auth pages are exempt from the `<h1>` rule, and only `lib/api.ts` may call `fetch`. The scopes are in `apps/web/eslint.config.mjs`.

**What lint cannot see**, so it stays your job and the review's: class names built from variables at run time, whether a screen has one primary action, the four states, status chips, the wording, focus order, and how the screen looks.

**Exceptions.** Use one only for a real exception, such as a value that is genuinely dynamic or a third party's brand mark, and write the reason after `--`:

```tsx
// eslint-disable-next-line swiftdrop/no-inline-style -- width comes from the API (a percentage)
<div style={{ width: `${progress}%` }} />
```

An `eslint-disable` added to make lint pass is a defect, and an unused one fails lint. If a rule is wrong, tell M2: the rule and its test change together.

**Changing the rules.** The rules are in `apps/web/eslint-rules/design-tokens.mjs` and their tests in `design-tokens.test.mjs`. Run the tests with:

```bash
node --test "apps/web/eslint-rules/*.test.mjs"
```

CI runs them too. When this file gains or changes a rule that can be checked, change the lint rule and its test in the same pull request.
