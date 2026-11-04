import nextEslintPluginNext from '@next/eslint-plugin-next';
import nx from '@nx/eslint-plugin';
import baseConfig from '../../eslint.config.mjs';
import swiftdrop from './eslint-rules/design-tokens.mjs';

/*
 * DESIGN.md as checks. The rules live in ./eslint-rules and are tested there.
 *
 * The design foundation defines the tokens, so it is exempt from the token
 * rules. Those files are protected (DESIGN.md section 14): a change to them
 * goes through the foundation owner, not through an eslint-disable comment.
 */
const sources = ['src/**/*.{ts,tsx}'];
const foundation = [
  'src/components/ui/**',
  'src/components/layout/**',
  'src/components/auth/brand-panel.tsx',
  'src/app/design/**',
  'src/app/layout.tsx',
  'src/lib/**',
];
/** Where screens are written: everything that is not the foundation. */
const screens = {
  files: sources,
  ignores: foundation,
};

const uiLibraries = [
  '@mui/*',
  '@chakra-ui/*',
  'antd',
  '@ant-design/*',
  '@mantine/*',
  '@nextui-org/*',
  '@heroui/*',
  '@blueprintjs/*',
  '@headlessui/*',
  '@radix-ui/*',
  'radix-ui',
  'react-bootstrap',
  'primereact',
  'semantic-ui-react',
];

const restrictedImports = {
  patterns: [
    {
      group: uiLibraries,
      message:
        'No UI component libraries. Use components/ui; if something is missing, ask the foundation owner (DESIGN.md sections 1 and 8).',
    },
    {
      group: [
        'react-icons',
        'react-icons/*',
        '@heroicons/*',
        '@fortawesome/*',
        '@tabler/*',
        '@phosphor-icons/*',
        'feather-icons',
        '@mui/icons-material',
      ],
      message:
        'Lucide is the only icon set: use lucide-react (DESIGN.md section 3).',
    },
    {
      group: [
        'styled-components',
        '@emotion/*',
        'styled-jsx',
        '@stitches/*',
        '@vanilla-extract/*',
        'twin.macro',
      ],
      message: 'No CSS-in-JS. Style with token classes (DESIGN.md section 4).',
    },
    {
      group: [
        'redux',
        'react-redux',
        '@reduxjs/toolkit',
        'zustand',
        'jotai',
        'recoil',
        'mobx',
        'mobx-react',
        'valtio',
        'xstate',
      ],
      message:
        'No state-management library without approval (DESIGN.md section 1).',
    },
    {
      group: [
        'swr',
        '@tanstack/react-query',
        'react-query',
        'axios',
        'ky',
        '@apollo/*',
      ],
      message:
        'Call the API with api() or apiWithMeta() from lib/api.ts, with no data-fetching library (DESIGN.md section 10).',
    },
    {
      group: ['next/font/google', 'next/font/local'],
      message:
        'Fonts are self-hosted once, in app/layout.tsx. Do not add fonts (DESIGN.md section 3).',
    },
  ],
};

/**
 * The foundation wraps Radix behind components/ui. Screens must never import
 * Radix directly, but the shadcn primitives themselves are allowed to.
 */
const foundationRestrictedImports = {
  patterns: [
    {
      group: uiLibraries.filter(
        (entry) => entry !== 'radix-ui' && entry !== '@radix-ui/*',
      ),
      message:
        'No UI component libraries. Use components/ui; if something is missing, ask the foundation owner (DESIGN.md sections 1 and 8).',
    },
    ...restrictedImports.patterns.slice(1),
  ],
};

const noErrorMessageOnScreen = {
  selector:
    "JSXExpressionContainer > MemberExpression[property.name='message'][object.name=/^(error|err|e)$/]",
  message:
    'Never show error.message to a person: it is written for developers. Decide by error.code and use our own words (DESIGN.md section 10).',
};

const noRawHeading = {
  selector: "JSXOpeningElement[name.name='h1']",
  message:
    'Do not write an h1 in a screen. PageHeader renders the page h1 and each Section an h2 (DESIGN.md section 6).',
};

export default [
  // A disable comment that no longer suppresses anything is itself an error.
  { linterOptions: { reportUnusedDisableDirectives: 'error' } },
  { plugins: { '@next/next': nextEslintPluginNext } },
  ...nx.configs['flat/react-typescript'],
  ...baseConfig,
  {
    ignores: ['.next/**/*', '**/out-tsc'],
  },

  // Everywhere in src: imports, the API client, and error text.
  {
    files: sources,
    rules: {
      'no-restricted-imports': ['error', restrictedImports],
      'no-restricted-globals': [
        'error',
        {
          name: 'fetch',
          message:
            'Call the API through api() or apiWithMeta() in lib/api.ts, never fetch directly (DESIGN.md section 10).',
        },
        {
          name: 'XMLHttpRequest',
          message:
            'Call the API through api() or apiWithMeta() in lib/api.ts (DESIGN.md section 10).',
        },
      ],
      'no-restricted-syntax': ['error', noErrorMessageOnScreen],
      'swiftdrop/no-raw-colors': 'error',
    },
    plugins: { swiftdrop },
  },

  // Screens: token classes only, no inline style, no raw h1.
  {
    ...screens,
    plugins: { swiftdrop },
    rules: {
      'swiftdrop/tailwind-tokens': 'error',
      'swiftdrop/no-inline-style': 'error',
      'no-restricted-syntax': ['error', noErrorMessageOnScreen, noRawHeading],
    },
  },

  // The auth pages are whole pages with their own layout, not shell screens.
  {
    files: [
      'src/components/auth/**',
      'src/app/(auth)/**',
      'src/app/not-found.tsx',
    ],
    rules: {
      'no-restricted-syntax': ['error', noErrorMessageOnScreen],
    },
  },

  // The shadcn primitives wrap Radix; screens still go through components/ui.
  {
    files: ['src/components/ui/**'],
    rules: { 'no-restricted-imports': ['error', foundationRestrictedImports] },
  },
  // The two places that are allowed to do what the rules above forbid.
  {
    files: ['src/lib/api.ts'],
    rules: { 'no-restricted-globals': 'off' },
  },
  {
    files: ['src/app/layout.tsx'],
    rules: { 'no-restricted-imports': 'off' },
  },
];
