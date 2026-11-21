/**
 * ESLint rules that turn DESIGN.md into checks, so a hex colour or an off-scale
 * spacing value fails `nx lint` (and CI) instead of waiting for review.
 *
 * Three rules, no dependencies:
 *   swiftdrop/tailwind-tokens  class strings may only use the design tokens
 *   swiftdrop/no-raw-colors    no hex or rgb()/hsl() colours in colour contexts
 *   swiftdrop/no-inline-style  no inline `style` prop
 *
 * The scopes (which files each rule covers) are set in eslint.config.mjs: the
 * design foundation (kit, shell, tokens) is exempt because it defines the
 * tokens. Rule tests: design-tokens.test.mjs.
 *
 * What the rules cannot see: class names built from variables at run time, and
 * class lists kept outside className / cn() / buttonClasses() calls. They catch
 * what is written out in the code, which is where the mistakes are made.
 */

const GUIDE = 'See DESIGN.md';

/** The spacing steps of the design system (4px base). 0 is also fine. */
const SPACE_STEPS = new Set([
  '0',
  '1',
  '2',
  '3',
  '4',
  '5',
  '6',
  '8',
  '10',
  '12',
  '16',
  '20',
  '24',
]);

/** Functions whose arguments are class lists. */
const CLASS_FUNCTIONS = new Set([
  'cn',
  'clsx',
  'classNames',
  'twMerge',
  'buttonClasses',
  'controlClasses',
  'groupInputClasses',
]);

const PALETTE =
  '(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)';
const COLOUR_UTILITIES =
  '(?:bg|text|border|border-[xytblrse]|ring|ring-offset|outline|fill|stroke|decoration|accent|caret|from|via|to|divide|placeholder|shadow)';
const DEFAULT_COLOUR = new RegExp(
  `^${COLOUR_UTILITIES}-(?:${PALETTE}(?:-\\d{2,3})?|white|black)(?:/\\d+)?$`,
);
const DEFAULT_TEXT_SIZE = /^text-(?:xs|sm|base|lg|xl|[2-9]xl)(?:\/.+)?$/;
const DEFAULT_RADIUS =
  /^rounded(?:-(?:t|b|l|r|s|e|tl|tr|bl|br|ss|se|es|ee))?(?:-(?:sm|md|lg|xl|2xl|3xl|4xl))?$/;
const SHADOW = /^(?:shadow|drop-shadow|inset-shadow)(?:-(.+))?$/;
const SPACING =
  /^(-?)(p|px|py|pt|pr|pb|pl|ps|pe|m|mx|my|mt|mr|mb|ml|ms|me|gap|gap-x|gap-y|space-x|space-y)-(\d+(?:\.\d+)?)$/;
const INK_FILL =
  /^(?:bg|border|ring|outline|from|via|to|divide)-ink(?:\/\d+)?$/;
const PAPER_TEXT =
  /^(?:text|fill|stroke|decoration|caret|placeholder)-paper(?:\/\d+)?$/;

/** An arbitrary value is fine only when it points at a token. */
const TOKEN_VARIABLE = /^\[var\(--[\w-]+(?:,[^)]*)?\)\]$/;
/** Layout structure, not a design value: custom grid tracks. */
const STRUCTURAL_ARBITRARY = /^(?:grid-cols|grid-rows)-\[/;

/**
 * Splits `md:hover:-mt-2` into its variants and its utility, ignoring colons
 * inside brackets or parentheses (`data-[state=open]:`, `[&:hover]:`).
 */
function splitToken(token) {
  let depth = 0;
  let last = -1;
  for (let i = 0; i < token.length; i += 1) {
    const char = token[i];
    if (char === '[' || char === '(') depth += 1;
    else if (char === ']' || char === ')') depth -= 1;
    else if (char === ':' && depth === 0) last = i;
  }
  const variants = last === -1 ? [] : topLevelSplit(token.slice(0, last));
  let utility = token.slice(last + 1);
  utility = utility.replace(/^!/, '').replace(/!$/, '');
  return { variants, utility };
}

function topLevelSplit(text) {
  const parts = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < text.length; i += 1) {
    const char = text[i];
    if (char === '[' || char === '(') depth += 1;
    else if (char === ']' || char === ')') depth -= 1;
    else if (char === ':' && depth === 0) {
      parts.push(text.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(text.slice(start));
  return parts;
}

/** Every problem in one class token, as { messageId, data }. */
function problemsInToken(token) {
  const problems = [];
  const { variants, utility } = splitToken(token);

  for (const variant of variants) {
    const name = variant.replace(/^(?:min|max)-/, '').replace(/\/.+$/, '');
    if (/^\d*xl$/.test(name) && name !== 'xl') {
      problems.push({ messageId: 'breakpoint', data: { token } });
    }
  }

  const arbitrary = /^[a-z][\w-]*-(\[.*\])$/.exec(utility);
  if (arbitrary || utility.startsWith('[')) {
    const bracket = arbitrary ? arbitrary[1] : utility;
    if (!TOKEN_VARIABLE.test(bracket) && !STRUCTURAL_ARBITRARY.test(utility)) {
      problems.push({ messageId: 'arbitrary', data: { token } });
    }
    return problems;
  }

  if (DEFAULT_COLOUR.test(utility)) {
    problems.push({ messageId: 'defaultColour', data: { token } });
  } else if (DEFAULT_TEXT_SIZE.test(utility)) {
    problems.push({ messageId: 'defaultSize', data: { token } });
  } else if (DEFAULT_RADIUS.test(utility)) {
    problems.push({ messageId: 'defaultRadius', data: { token } });
  } else if (INK_FILL.test(utility) || PAPER_TEXT.test(utility)) {
    problems.push({ messageId: 'inkPaper', data: { token } });
  } else {
    const shadow = SHADOW.exec(utility);
    if (shadow) {
      const name = shadow[1];
      const isToken =
        utility.startsWith('shadow') &&
        /^(popover|modal|none)$/.test(name ?? '');
      if (!isToken) problems.push({ messageId: 'shadow', data: { token } });
    } else {
      const spacing = SPACING.exec(utility);
      if (spacing && !SPACE_STEPS.has(spacing[3])) {
        problems.push({ messageId: 'spacing', data: { token } });
      }
    }
  }
  return problems;
}

/** Collects every string in an expression: literals and template pieces. */
function collectStrings(node, out) {
  if (!node || typeof node.type !== 'string') return;
  if (node.type === 'Literal' && typeof node.value === 'string') {
    out.push({ node, text: node.value });
  } else if (node.type === 'TemplateElement') {
    out.push({ node, text: node.value.cooked ?? node.value.raw });
  }
  for (const key of Object.keys(node)) {
    if (key === 'parent') continue;
    const value = node[key];
    if (Array.isArray(value))
      value.forEach((child) => collectStrings(child, out));
    else if (value && typeof value.type === 'string')
      collectStrings(value, out);
  }
}

function isClassAttribute(name) {
  return (
    name === 'className' ||
    /[a-z]ClassName$/.test(name) ||
    name === 'classNames'
  );
}

const tailwindTokens = {
  meta: {
    type: 'problem',
    schema: [],
    docs: {
      description:
        'Class lists may only use the SwiftDrop design tokens (DESIGN.md section 4).',
    },
    messages: {
      arbitrary: `"{{token}}" is an arbitrary value. Use a design token class instead. ${GUIDE} section 4.`,
      defaultColour: `"{{token}}" is a default Tailwind colour, removed in this project. Use a token such as bg-surface or text-text-muted. ${GUIDE} section 4.`,
      defaultSize: `"{{token}}" is a default Tailwind text size, removed in this project. Use text-body, text-body-sm, text-label, text-caption or text-h1 to text-h4. ${GUIDE} section 4.`,
      defaultRadius: `"{{token}}" is not in the radius scale. Use rounded-chip, rounded-control, rounded-card or rounded-full. ${GUIDE} section 4.`,
      shadow: `"{{token}}": only popovers and modals have a shadow (shadow-popover, shadow-modal). Separate things with border border-line instead (design rule 02). ${GUIDE} section 2.`,
      spacing: `"{{token}}" is off the spacing scale. Use 0, 1, 2, 3, 4, 5, 6, 8, 10, 12, 16, 20 or 24 for padding, margin and gap. ${GUIDE} section 4.`,
      inkPaper: `"{{token}}" does not flip in dark mode (ink and paper behave differently from the semantic tokens). Use bg-primary text-on-primary or the surface tokens. ${GUIDE} section 3.`,
      breakpoint: `"{{token}}" uses a breakpoint that does not exist here. The breakpoints are sm, md, lg and xl. ${GUIDE} section 4.`,
    },
  },
  create(context) {
    const seen = new WeakSet();

    function check(root) {
      const strings = [];
      collectStrings(root, strings);
      for (const { node, text } of strings) {
        if (seen.has(node)) continue;
        seen.add(node);
        for (const token of text.split(/\s+/).filter(Boolean)) {
          for (const problem of problemsInToken(token)) {
            context.report({ node, ...problem });
          }
        }
      }
    }

    return {
      JSXAttribute(node) {
        if (
          node.name.type === 'JSXIdentifier' &&
          isClassAttribute(node.name.name)
        ) {
          check(node.value);
        }
      },
      CallExpression(node) {
        if (
          node.callee.type === 'Identifier' &&
          CLASS_FUNCTIONS.has(node.callee.name)
        ) {
          node.arguments.forEach(check);
        }
      },
    };
  },
};

/**
 * A hex colour, or a colour function. Tailwind writes spaces as underscores in
 * arbitrary values, so `_` counts as a separator here, and `color-mix()` of a
 * token is fine (it is how a token gets an alpha).
 */
const HEX_COLOUR =
  /(?<![A-Za-z0-9&-])(?<!url\()#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})(?![A-Za-z0-9-])/;
const COLOUR_FUNCTION = /\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(/;
const COLOUR_ATTRIBUTES = new Set([
  'fill',
  'stroke',
  'stopColor',
  'floodColor',
  'lightingColor',
  'color',
]);
const COLOUR_KEYS = new Set([
  'color',
  'background',
  'backgroundColor',
  'borderColor',
  'outlineColor',
  'fill',
  'stroke',
  'boxShadow',
  'textShadow',
]);

function hasRawColour(text) {
  return HEX_COLOUR.test(text) || COLOUR_FUNCTION.test(text);
}

const noRawColors = {
  meta: {
    type: 'problem',
    schema: [],
    docs: {
      description:
        'No hex, rgb() or hsl() colours: use a colour token (design rule 06).',
    },
    messages: {
      rawColour: `Raw colour "{{value}}". Every colour comes from the token set (design rule 06). Use a token class such as text-text-muted or bg-surface. ${GUIDE} section 2.`,
    },
  },
  create(context) {
    const seen = new WeakSet();

    function check(root) {
      const strings = [];
      collectStrings(root, strings);
      for (const { node, text } of strings) {
        if (seen.has(node)) continue;
        seen.add(node);
        if (hasRawColour(text)) {
          context.report({
            node,
            messageId: 'rawColour',
            data: { value: text.trim().slice(0, 40) },
          });
        }
      }
    }

    return {
      JSXAttribute(node) {
        if (node.name.type !== 'JSXIdentifier') return;
        const name = node.name.name;
        if (
          COLOUR_ATTRIBUTES.has(name) ||
          name === 'style' ||
          isClassAttribute(name)
        ) {
          check(node.value);
        }
      },
      Property(node) {
        const key =
          node.key.type === 'Identifier' ? node.key.name : node.key.value;
        if (COLOUR_KEYS.has(key)) check(node.value);
      },
      CallExpression(node) {
        if (
          node.callee.type === 'Identifier' &&
          CLASS_FUNCTIONS.has(node.callee.name)
        ) {
          node.arguments.forEach(check);
        }
      },
    };
  },
};

const noInlineStyle = {
  meta: {
    type: 'problem',
    schema: [],
    docs: { description: 'No inline style prop: use token classes.' },
    messages: {
      inlineStyle: `Do not style with an inline style prop. Use token classes (design rule 06). If a value really is dynamic, add a short eslint-disable comment that says why. ${GUIDE} section 4.`,
    },
  },
  create(context) {
    return {
      JSXAttribute(node) {
        if (node.name.type === 'JSXIdentifier' && node.name.name === 'style') {
          context.report({ node, messageId: 'inlineStyle' });
        }
      },
    };
  },
};

export default {
  meta: { name: 'swiftdrop' },
  rules: {
    'tailwind-tokens': tailwindTokens,
    'no-raw-colors': noRawColors,
    'no-inline-style': noInlineStyle,
  },
};

export { problemsInToken, splitToken };
