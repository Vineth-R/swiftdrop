import { describe, expect, it, vi } from 'vitest';

import {
  emitSessionIdentity,
  getSessionIdentityVersion,
  subscribeSessionIdentity,
} from './session-events';

describe('session identity version', () => {
  it('increments and notifies only when the identity changes', () => {
    const before = getSessionIdentityVersion();
    const listener = vi.fn();
    const stop = subscribeSessionIdentity(listener);

    emitSessionIdentity('user-a');
    emitSessionIdentity('user-a');
    expect(listener).toHaveBeenCalledTimes(1);
    expect(getSessionIdentityVersion()).toBe(before + 1);

    emitSessionIdentity(null);
    expect(listener).toHaveBeenCalledTimes(2);
    expect(getSessionIdentityVersion()).toBe(before + 2);

    stop();
    emitSessionIdentity('user-b');
    expect(listener).toHaveBeenCalledTimes(2);
  });
});
