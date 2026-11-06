import type { Locator } from '@playwright/test';

// This project has no DOM types, but these run in the browser, where the globals
// exist. Declaring them here keeps the functions below real functions, which
// Playwright runs reliably (a function written as a string is guessed at).
declare const getComputedStyle: (element: unknown) => Record<string, string>;

type Matchable = { matches(selector: string): boolean };

/** One computed style of an element, such as `color` or `backgroundColor`. */
export async function computed(
  locator: Locator,
  property: string,
): Promise<string> {
  return locator.evaluate(
    (element, name) => getComputedStyle(element)[name],
    property,
  );
}

/** Whether the element matches a selector, such as `:modal` for an open modal dialog. */
export async function matches(
  locator: Locator,
  selector: string,
): Promise<boolean> {
  return locator.evaluate(
    (element, query) => (element as unknown as Matchable).matches(query),
    selector,
  );
}
