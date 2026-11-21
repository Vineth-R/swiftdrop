/**
 * Tests for the design-token lint rules. They run on Node's built-in test
 * runner with ESLint's own RuleTester, so they add no dependency:
 *
 *   node --test "apps/web/eslint-rules/*.test.mjs"
 */
import { describe, it } from 'node:test';
import { RuleTester } from 'eslint';

import plugin, { problemsInToken, splitToken } from './design-tokens.mjs';

RuleTester.describe = describe;
RuleTester.it = it;
RuleTester.itOnly = it.only;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    parserOptions: { ecmaFeatures: { jsx: true } },
  },
});

const jsx = (className) => `const a = <div className="${className}" />;`;
const errors = (...ids) => ids.map((messageId) => ({ messageId }));

describe('splitToken', () => {
  it('separates variants from the utility, ignoring colons inside brackets', () => {
    const cases = [
      ['p-4', [], 'p-4'],
      ['md:hover:-mt-2', ['md', 'hover'], '-mt-2'],
      ['data-[state=open]:bg-surface', ['data-[state=open]'], 'bg-surface'],
      ['[&:hover]:p-7', ['[&:hover]'], 'p-7'],
      ['!p-4', [], 'p-4'],
    ];
    for (const [token, variants, utility] of cases) {
      const result = splitToken(token);
      if (
        JSON.stringify(result.variants) !== JSON.stringify(variants) ||
        result.utility !== utility
      ) {
        throw new Error(`${token} split into ${JSON.stringify(result)}`);
      }
    }
  });
});

describe('problemsInToken', () => {
  it('accepts every class the kit really uses', () => {
    const fine = [
      'flex',
      'items-center',
      'gap-3',
      'p-4',
      'px-5',
      'py-3',
      'mt-6',
      '-mt-2',
      'mx-auto',
      'gap-x-4',
      'space-y-8',
      'md:p-10',
      'lg:gap-12',
      'hover:bg-surface-2',
      'focus-visible:outline-2',
      'bg-surface',
      'bg-primary',
      'text-on-primary',
      'text-text-muted',
      'border-line',
      'border-danger-border',
      'bg-paper',
      'bg-transparent',
      'text-current',
      'border-transparent',
      'bg-accent',
      'text-status-in-transit-fg',
      'text-h1',
      'text-body-sm',
      'text-caption',
      'text-overline',
      'text-button',
      'text-body-lg',
      'font-display',
      'font-mono',
      'font-semibold',
      'uppercase',
      'tabular',
      'rounded-chip',
      'rounded-control',
      'rounded-card',
      'rounded-full',
      'rounded-none',
      'rounded-t-card',
      'shadow-popover',
      'shadow-modal',
      'shadow-none',
      'size-14',
      'h-12',
      'w-full',
      'max-w-content',
      'max-w-xl',
      'min-h-screen',
      'z-20',
      'top-0',
      'grid-cols-[8rem_repeat(5,minmax(0,1fr))]',
      'p-[var(--panel-padding)]',
      'bg-[var(--color-surface)]',
      'p-0',
      'p-px',
      'm-auto',
      'sm:p-4',
      'xl:p-6',
      'max-md:hidden',
    ];
    for (const token of fine) {
      const found = problemsInToken(token);
      if (found.length)
        throw new Error(
          `${token} should be fine but gave ${found.map((p) => p.messageId)}`,
        );
    }
  });

  it('flags every kind of violation', () => {
    const bad = [
      ['p-[13px]', 'arbitrary'],
      ['bg-[#fff]', 'arbitrary'],
      ['w-[317px]', 'arbitrary'],
      ['md:text-[15px]', 'arbitrary'],
      ['[mask-type:luminance]', 'arbitrary'],
      ['bg-red-500', 'defaultColour'],
      ['text-slate-600', 'defaultColour'],
      ['border-t-blue-200', 'defaultColour'],
      ['bg-white', 'defaultColour'],
      ['text-black/50', 'defaultColour'],
      ['text-sm', 'defaultSize'],
      ['md:text-lg', 'defaultSize'],
      ['text-2xl', 'defaultSize'],
      ['rounded', 'defaultRadius'],
      ['rounded-lg', 'defaultRadius'],
      ['rounded-t-xl', 'defaultRadius'],
      ['shadow', 'shadow'],
      ['shadow-md', 'shadow'],
      ['drop-shadow-lg', 'shadow'],
      ['p-7', 'spacing'],
      ['gap-9', 'spacing'],
      ['md:px-1.5', 'spacing'],
      ['-mt-7', 'spacing'],
      ['space-y-14', 'spacing'],
      ['bg-ink', 'inkPaper'],
      ['border-ink', 'inkPaper'],
      ['text-paper', 'inkPaper'],
      ['2xl:p-4', 'breakpoint'],
    ];
    for (const [token, messageId] of bad) {
      const found = problemsInToken(token).map((p) => p.messageId);
      if (!found.includes(messageId))
        throw new Error(
          `${token} should give ${messageId} but gave [${found}]`,
        );
    }
  });
});

tester.run('swiftdrop/tailwind-tokens', plugin.rules['tailwind-tokens'], {
  valid: [
    {
      code: jsx(
        'flex items-center gap-3 p-4 bg-surface text-text-muted rounded-control',
      ),
    },
    {
      code: 'const a = <div className={cn("p-4", isOpen && "bg-surface-2")} />;',
    },
    { code: 'const a = <div className={`p-4 ${gap}`} />;' },
    { code: 'const a = <div title="p-7 rounded-lg" />;' },
    { code: 'const a = "p-7 rounded-lg";' },
    { code: 'const a = <Card labelClassName="text-label" />;' },
    { code: 'const a = buttonClasses({ variant: "primary" });' },
  ],
  invalid: [
    { code: jsx('p-[13px]'), errors: errors('arbitrary') },
    {
      code: jsx('bg-red-500 text-sm'),
      errors: errors('defaultColour', 'defaultSize'),
    },
    {
      code: jsx('rounded-lg shadow-md'),
      errors: errors('defaultRadius', 'shadow'),
    },
    { code: jsx('p-7 gap-9'), errors: errors('spacing', 'spacing') },
    { code: jsx('bg-ink text-paper'), errors: errors('inkPaper', 'inkPaper') },
    {
      code: 'const a = <div className={cn("p-4", isOpen && "p-7")} />;',
      errors: errors('spacing'),
    },
    {
      code: 'const a = <div className={`flex ${x} rounded-lg`} />;',
      errors: errors('defaultRadius'),
    },
    {
      code: 'const a = <div className={ok ? "p-4" : "bg-blue-500"} />;',
      errors: errors('defaultColour'),
    },
    {
      code: 'const a = <Card labelClassName="text-xl" />;',
      errors: errors('defaultSize'),
    },
    { code: 'const classes = cn("flex", "gap-7");', errors: errors('spacing') },
    {
      code: 'const classes = buttonClasses() + cn("shadow-lg");',
      errors: errors('shadow'),
    },
    { code: jsx('2xl:p-4'), errors: errors('breakpoint') },
    { code: jsx('md:hover:p-[7px]'), errors: errors('arbitrary') },
  ],
});

tester.run('swiftdrop/no-raw-colors', plugin.rules['no-raw-colors'], {
  valid: [
    { code: 'const a = <div className="bg-surface" />;' },
    { code: 'const a = <a href="#main">Skip</a>;' },
    { code: 'const a = <p>Order #1024 is on its way</p>;' },
    { code: 'const a = <svg><use href="url(#abc)" /></svg>;' },
    { code: 'const a = <svg fill="currentColor" />;' },
    { code: 'const a = <div style={{ width: "50%" }} />;' },
    { code: 'const a = { label: "#1024" };' },
    { code: 'const a = <circle stroke="var(--color-line)" />;' },
    {
      code: 'const a = <div className="bg-[radial-gradient(color-mix(in_oklab,var(--color-accent)_6%,transparent),transparent)]" />;',
    },
  ],
  invalid: [
    { code: 'const a = <svg fill="#4285F4" />;', errors: errors('rawColour') },
    { code: 'const a = <path stroke="#fff" />;', errors: errors('rawColour') },
    {
      code: 'const a = <div style={{ color: "#c23b2e" }} />;',
      errors: errors('rawColour'),
    },
    {
      code: 'const a = <div style={{ background: "rgb(20 23 26)" }} />;',
      errors: errors('rawColour'),
    },
    {
      code: 'const a = { backgroundColor: "hsl(0 0% 100%)" };',
      errors: errors('rawColour'),
    },
    {
      code: 'const a = { boxShadow: "0 1px 2px rgba(0,0,0,.1)" };',
      errors: errors('rawColour'),
    },
    {
      code: 'const a = <div className="bg-[#ffffff]" />;',
      errors: errors('rawColour'),
    },
    { code: 'const a = <svg color="#14171a" />;', errors: errors('rawColour') },
    {
      code: 'const a = <div className="[mask-image:linear-gradient(#000_25%,transparent)]" />;',
      errors: errors('rawColour'),
    },
    {
      code: 'const a = <div className="bg-[linear-gradient(to_right,_#fff_0,_rgb(0_0_0))]" />;',
      errors: errors('rawColour'),
    },
  ],
});

tester.run('swiftdrop/no-inline-style', plugin.rules['no-inline-style'], {
  valid: [
    { code: 'const a = <div className="p-4" />;' },
    { code: 'const a = <div data-style="x" />;' },
  ],
  invalid: [
    {
      code: 'const a = <div style={{ width: "50%" }} />;',
      errors: errors('inlineStyle'),
    },
    {
      code: 'const a = <div style={styles} />;',
      errors: errors('inlineStyle'),
    },
  ],
});
