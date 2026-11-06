import type { Metadata } from 'next';

import { ThemeSwitch } from '@/components/ui/theme-switch';
import { ComponentsSection } from './components-section';
import { KitMoreSection } from './kit-more-section';
import { KitSection } from './kit-section';
import { ShadcnSection } from './shadcn-section';
import {
  colorTokens,
  durationTokens,
  radiusTokens,
  shadowTokens,
  spaceScale,
  textScale,
} from '@/lib/tokens';

export const metadata: Metadata = {
  title: 'Design tokens — SwiftDrop',
  description: 'The SwiftDrop design tokens, in the active theme.',
};

const radiusClass = {
  chip: 'rounded-chip',
  control: 'rounded-control',
  card: 'rounded-card',
  splash: 'rounded-splash',
} as const;

const shadowClass = {
  popover: 'shadow-popover',
  modal: 'shadow-modal',
} as const;

const durationClass = {
  fast: 'duration-fast',
  base: 'duration-base',
  slow: 'duration-slow',
} as const;

function Overline({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-overline uppercase text-text-faint">{children}</h2>
  );
}

export default function DesignPage() {
  return (
    <main className="mx-auto flex max-w-content flex-col gap-12 px-4 py-12 md:px-8">
      <header className="flex flex-wrap items-end justify-between gap-6">
        <div className="flex flex-col gap-2">
          <Overline>Style guide</Overline>
          <h1 className="font-display text-h1">Design tokens</h1>
          <p className="max-w-prose text-body text-text-muted">
            Every value below comes from{' '}
            <code className="font-mono">globals.css</code>. Switch the theme to
            see the dark values replace the light ones.
          </p>
        </div>
        <ThemeSwitch />
      </header>

      <section aria-labelledby="colour" className="flex flex-col gap-6">
        <h2 id="colour" className="font-display text-h2">
          Colour
        </h2>
        {Object.entries(colorTokens).map(([group, tokens]) => (
          <div key={group} className="flex flex-col gap-3">
            <Overline>{group}</Overline>
            <ul className="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-6">
              {tokens.map((token) => (
                <li
                  key={token}
                  className="flex flex-col gap-2 rounded-card border border-line bg-surface p-3"
                >
                  <span
                    className="h-10 rounded-chip border border-line"
                    style={{ background: `var(--color-${token})` }}
                    aria-hidden="true"
                  />
                  <span className="font-mono text-caption break-all">
                    {token}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </section>

      <section aria-labelledby="type" className="flex flex-col gap-6">
        <h2 id="type" className="font-display text-h2">
          Type
        </h2>
        <ul className="flex flex-col divide-y divide-line rounded-card border border-line bg-surface">
          {textScale.map(({ token, className, spec }) => (
            <li
              key={token}
              className="flex flex-wrap items-baseline justify-between gap-4 px-5 py-4"
            >
              <span className={className}>SwiftDrop {token}</span>
              <span className="font-mono text-caption text-text-faint">
                {token} · {spec}
              </span>
            </li>
          ))}
          <li className="flex flex-wrap items-baseline justify-between gap-4 px-5 py-4">
            <span className="tabular font-mono text-label">
              SDK7M2Q9XP4 · 550.00
            </span>
            <span className="font-mono text-caption text-text-faint">
              mono · tabular
            </span>
          </li>
        </ul>
      </section>

      <section aria-labelledby="space" className="flex flex-col gap-6">
        <h2 id="space" className="font-display text-h2">
          Space, radius, shadow, motion
        </h2>
        <div className="grid gap-6 md:grid-cols-2">
          <div className="flex flex-col gap-3 rounded-card border border-line bg-surface p-6">
            <Overline>Spacing, 4px base</Overline>
            <ul className="flex flex-col gap-2">
              {spaceScale.map((step) => (
                <li key={step} className="flex items-center gap-3">
                  <span className="w-16 font-mono text-caption text-text-faint">
                    {step} · {step * 4}
                  </span>
                  <span
                    className="h-3 bg-ink"
                    style={{ width: `calc(var(--spacing) * ${step})` }}
                  />
                </li>
              ))}
            </ul>
          </div>
          <div className="flex flex-col gap-8 rounded-card border border-line bg-surface p-6">
            <div className="flex flex-col gap-3">
              <Overline>Radius</Overline>
              <ul className="flex flex-wrap gap-4">
                {radiusTokens.map((token) => (
                  <li key={token} className="flex flex-col items-center gap-2">
                    <span
                      className={`h-11 w-16 border border-line-control bg-surface-2 ${radiusClass[token]}`}
                    />
                    <span className="font-mono text-caption text-text-faint">
                      {token}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
            <div className="flex flex-col gap-3">
              <Overline>Elevation</Overline>
              <ul className="flex flex-wrap gap-4">
                {shadowTokens.map((token) => (
                  <li key={token} className="flex flex-col items-center gap-2">
                    <span
                      className={`h-16 w-28 rounded-card border border-line bg-surface ${shadowClass[token]}`}
                    />
                    <span className="font-mono text-caption text-text-faint">
                      shadow-{token}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
            <div className="flex flex-col gap-3">
              <Overline>Motion</Overline>
              <ul className="flex flex-col gap-1 font-mono text-caption text-text-faint">
                {durationTokens.map((token) => (
                  <li key={token} className={durationClass[token]}>
                    duration-{token}
                  </li>
                ))}
                <li>ease-out-sd · cubic-bezier(.2,.8,.2,1)</li>
              </ul>
            </div>
          </div>
        </div>
      </section>

      <ComponentsSection />

      <KitSection />

      <KitMoreSection />

      <ShadcnSection />

      <section aria-labelledby="focus" className="flex flex-col gap-4">
        <h2 id="focus" className="font-display text-h2">
          Focus
        </h2>
        <p className="max-w-prose text-body text-text-muted">
          One rule for the whole app: a 2px interactive outline with 2px offset.
          Tab to this link to see it.
        </p>
        <a
          href="#colour"
          className="w-fit text-body text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Back to colour
        </a>
      </section>
    </main>
  );
}
