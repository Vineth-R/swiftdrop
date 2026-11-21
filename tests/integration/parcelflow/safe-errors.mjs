/** Prevent Playwright's HTTP diagnostics from printing local session headers. */
export function withSafeErrors(run) {
  return async (...args) => {
    try {
      return await run(...args);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Integration check failed';
      const safe = message
        .split(/\r?\nCall log:/i)[0]
        .replace(
          /\b(authorization|cookie|set-cookie|x-internal-token)\s*[:=][^\r\n]*/gi,
          '$1: [REDACTED]',
        );
      // Do not retain the original stack, cause, actual or expected properties:
      // assertion/request diagnostics can carry cookies even after redaction.
      throw new Error(safe);
    }
  };
}
