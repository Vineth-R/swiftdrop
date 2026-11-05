/**
 * Lets the lib/*-api tests run on Node's built-in test runner, like the design
 * lint rules, so they add no dependency and need no browser:
 *
 *   node --test "apps/web/src/lib/*.test.mjs"
 *
 * Importing this file teaches Node to load our TypeScript. The tests call the
 * real functions and answer `fetch` here, so they see exactly what is sent.
 */
import { readFileSync } from 'node:fs';
import { registerHooks } from 'node:module';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

registerHooks({
  resolve(specifier, context, next) {
    const hasNoExtension =
      specifier.startsWith('./') && !/\.[a-z]+$/.test(specifier);
    return next(hasNoExtension ? `${specifier}.ts` : specifier, context);
  },
  load(url, context, next) {
    if (!url.endsWith('.ts')) return next(url, context);
    const { outputText } = ts.transpileModule(
      readFileSync(fileURLToPath(url), 'utf8'),
      {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      },
    );
    return { format: 'module', source: outputText, shortCircuit: true };
  },
});

const realFetch = globalThis.fetch;

export function restoreFetch() {
  globalThis.fetch = realFetch;
}

/** Answers every call with this status and body. Returns the calls it saw. */
export function stubFetch(status, body, headers) {
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push({
      url: String(url),
      method: init.method,
      body: init.body ? JSON.parse(init.body) : undefined,
      signal: init.signal,
    });
    return new Response(JSON.stringify(body), { status, headers });
  };
  return calls;
}

export const ok = (data) => stubFetch(200, { data, meta: {} });

/** An API failure. Its message must never decide what a screen does. */
export const fail = (status, code, headers) =>
  stubFetch(
    status,
    { error: { code, message: 'RAW SERVER WORDS' }, meta: {} },
    headers,
  );

export function failNetwork() {
  globalThis.fetch = async () => {
    throw new TypeError('Failed to fetch');
  };
}
