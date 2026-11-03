import { Lock, Plus } from 'lucide-react';

import { SystemPage } from '../../components/layout/system-page';
import { Alert } from '../../components/ui/alert';
import { Avatar } from '../../components/ui/avatar';
import { Banner } from '../../components/ui/banner';
import { Button, ButtonLink } from '../../components/ui/button';
import { ErrorSummary } from '../../components/ui/error-summary';
import { TextField } from '../../components/ui/field';
import { Logo, LogoMark } from '../../components/ui/logo';
import { PasswordField } from '../../components/ui/password-field';
import { RoleBadge } from '../../components/ui/role-badge';
import { RouteLine } from '../../components/ui/route-line';
import { Spinner } from '../../components/ui/spinner';
import type { ButtonVariant } from '../../components/ui/button';
import { roles } from '../../lib/tokens';

const variants: { variant: ButtonVariant; label: string; note: string }[] = [
  { variant: 'primary', label: 'Create', note: 'Ink fill' },
  { variant: 'secondary', label: 'Cancel', note: 'Outlined' },
  { variant: 'accent', label: 'Track', note: 'Marketing only' },
  { variant: 'ghost', label: 'Filter', note: 'Toolbars, rows' },
  { variant: 'danger', label: 'Revoke', note: 'Destructive' },
  { variant: 'link', label: 'Resend', note: 'Inline, teal' },
];

function Overline({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="text-overline uppercase text-text-faint">{children}</h3>
  );
}

function Panel({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6">
      <Overline>{title}</Overline>
      {children}
    </div>
  );
}

export function ComponentsSection() {
  return (
    <section aria-labelledby="components" className="flex flex-col gap-8">
      <h2 id="components" className="font-display text-h2">
        Components
      </h2>

      <div className="grid gap-6 md:grid-cols-2">
        <Panel title="Logo">
          <div className="flex flex-wrap items-center gap-6">
            <Logo size="lg" />
            <Logo />
            <Logo size="sm" />
          </div>
          <div className="flex items-end gap-4">
            <LogoMark size={48} />
            <LogoMark size={32} />
            <LogoMark size={76} radius={12} />
          </div>
        </Panel>
        <Panel title="Route line and spinner">
          <div className="flex flex-col gap-4">
            <RouteLine />
            <RouteLine state="travelling" />
            <RouteLine state="complete" />
          </div>
          <div className="flex items-center gap-4">
            <Spinner size={20} tone="standalone" label="Loading" />
            <Spinner />
            <span className="text-body-sm text-text-muted">
              The node runs the ring instead of an arc sweep.
            </span>
          </div>
          <div className="flex h-24 gap-4">
            <RouteLine
              orientation="vertical"
              state="travelling"
              nodeSize={10}
            />
            <span className="text-body-sm text-text-muted">
              Vertical, for the login panel.
            </span>
          </div>
        </Panel>
      </div>

      <Panel title="Buttons">
        <div className="grid gap-x-6 gap-y-5 md:grid-cols-[8rem_repeat(5,minmax(0,1fr))]">
          {[
            'Variant',
            'Default',
            'Disabled',
            'Loading',
            'Full width',
            'Icon',
          ].map((heading) => (
            <span
              key={heading}
              className="hidden text-overline uppercase text-text-faint md:block"
            >
              {heading}
            </span>
          ))}
          {variants.map(({ variant, label, note }) => (
            <div key={variant} className="contents">
              <div className="flex flex-col">
                <span className="text-label font-semibold capitalize">
                  {variant}
                </span>
                <span className="text-caption text-text-faint">{note}</span>
              </div>
              <div>
                <Button variant={variant}>{label}</Button>
              </div>
              <div>
                <Button variant={variant} isDisabled>
                  {label}
                </Button>
              </div>
              <div>
                <Button variant={variant} isLoading>
                  {label}
                </Button>
              </div>
              <div>
                <Button variant={variant} isFullWidth>
                  {label}
                </Button>
              </div>
              <div>
                <Button
                  variant={variant}
                  iconLeading={
                    <Plus size={18} strokeWidth={1.5} aria-hidden="true" />
                  }
                >
                  {label}
                </Button>
              </div>
            </div>
          ))}
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <Button size="sm">Small</Button>
          <Button>Medium</Button>
          <Button size="lg">Large</Button>
          <ButtonLink href="/design" variant="secondary">
            Link as button
          </ButtonLink>
        </div>
      </Panel>

      <div className="grid gap-6 md:grid-cols-2">
        <Panel title="Text fields">
          <TextField
            id="demo-name"
            label="Recipient name"
            placeholder="Nimali Perera"
            helper="2 to 150 characters. Shown to the courier."
          />
          <TextField
            id="demo-phone"
            label="Recipient phone"
            defaultValue="+94 71 234"
            error="Enter a full number, for example +94 71 234 5678."
            isRequired
          />
          <TextField
            id="demo-tracking"
            label="Tracking number"
            defaultValue="SDK7M2Q9XP4"
            success="Found — parcel is in transit to Kandy."
            isMono
          />
          <TextField
            id="demo-readonly"
            label="Sign-in provider"
            defaultValue="Google"
            readOnly
          />
          <TextField
            id="demo-disabled"
            label="Merchant ID"
            defaultValue="MRC-00418"
            disabled
            isMono
          />
          <TextField
            id="demo-amount"
            label="Declared value"
            prefix="LKR"
            suffix=".00"
            defaultValue="4,500"
            inputMode="decimal"
            isMono
            helper="Amounts are mono, right-aligned, two decimals."
          />
        </Panel>

        <div className="flex flex-col gap-6">
          <Panel title="Password">
            <PasswordField
              id="demo-new-password"
              label="New password"
              autoComplete="new-password"
              defaultValue="Colombo26"
              showChecklist
            />
          </Panel>
          <Panel title="Password, plain">
            <PasswordField
              id="demo-password"
              label="Password"
              autoComplete="current-password"
              controlSize="lg"
            />
          </Panel>
        </div>
      </div>

      <Panel title="Alerts">
        <Alert tone="info" title="Pricing for Jaffna changes on 15 November">
          Parcels created before then keep the current rate.
        </Alert>
        <Alert tone="success" title="Proof of delivery saved">
          SDK7M2Q9XP4 is now marked DELIVERED. The customer has been emailed.
        </Alert>
        <Alert
          tone="warning"
          title="Two couriers in the Galle zone are over capacity"
        >
          Reassign six parcels, or the evening run will spill to tomorrow.
        </Alert>
        <Alert
          tone="danger"
          title="Delivery failed — recipient not at the address"
          actions={
            <>
              <Button size="sm">Reschedule</Button>
              <Button size="sm" variant="secondary">
                Return to hub
              </Button>
            </>
          }
        >
          The courier attempted delivery at 16:42.
        </Alert>
        <Alert tone="danger" icon={Lock} title="Account locked for 15 minutes">
          Five failed attempts. Try again later, or reset your password now.
        </Alert>
      </Panel>

      <div className="grid gap-6 md:grid-cols-2">
        <Panel title="Role badges and avatar">
          <div className="flex flex-wrap items-center gap-3">
            {roles.map((role) => (
              <RoleBadge key={role} role={role} />
            ))}
          </div>
          <div className="flex items-center gap-3">
            <Avatar name="Nimali Perera" />
            <Avatar name="Kasun" />
            <Avatar name="Anusha Nawarathna" size="sm" />
          </div>
        </Panel>
        <Panel title="Banners">
          <div className="overflow-hidden rounded-card border border-line">
            <Banner
              tone="warning"
              title="Merchant account pending approval."
              action={
                <Button size="sm" variant="secondary">
                  Check status
                </Button>
              }
            >
              Orders are accepted once an admin approves the account.
            </Banner>
            <Banner tone="info" title="Scheduled maintenance 02:00 to 03:00.">
              Tracking stays available.
            </Banner>
            <Banner tone="danger" title="Your account is deactivated.">
              Contact support to reactivate.
            </Banner>
          </div>
        </Panel>
      </div>

      <Panel title="System page (shown inside the shell for 404, 403 and 500)">
        <div className="rounded-card border border-line bg-paper px-6">
          <SystemPage
            label="403 · Forbidden"
            title="You do not have access"
            actions={<Button variant="secondary">Back to my dashboard</Button>}
          >
            This page is for administrators. You are signed in as a dispatcher,
            which is probably correct: nothing is wrong with your account.
          </SystemPage>
        </div>
      </Panel>

      <Panel title="Form-level error summary">
        <ErrorSummary
          focusOnMount={false}
          requestId="7f2c9a14-0b63-4de8-9f10-5c2e8d3a1b77"
          issues={[
            {
              fieldId: 'demo-phone',
              label: 'Recipient phone',
              message: 'enter a full number, for example +94 71 234 5678.',
            },
            {
              fieldId: 'demo-name',
              label: 'Recipient name',
              message: 'enter between 2 and 150 characters.',
            },
          ]}
        />
      </Panel>
    </section>
  );
}
