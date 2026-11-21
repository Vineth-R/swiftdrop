import { describe, expect, it, vi } from 'vitest';

import {
  emitSessionIdentity,
  getSessionIdentityVersion,
  getSessionRole,
  subscribeSessionIdentity,
} from './session-events';

describe('session identity version', () => {
  it('increments and notifies only when the identity changes', () => {
    const before = getSessionIdentityVersion();
    const listener = vi.fn();
    const stop = subscribeSessionIdentity(listener);

    emitSessionIdentity('user-a', 'CUSTOMER');
    emitSessionIdentity('user-a', 'CUSTOMER');
    expect(listener).toHaveBeenCalledTimes(1);
    expect(getSessionIdentityVersion()).toBe(before + 1);
    expect(getSessionRole()).toBe('CUSTOMER');

    emitSessionIdentity(null, null);
    expect(listener).toHaveBeenCalledTimes(2);
    expect(getSessionIdentityVersion()).toBe(before + 2);
    expect(getSessionRole()).toBeNull();

    stop();
    emitSessionIdentity('user-b', 'ADMIN');
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it('updates a corrected role without resetting the identity version', () => {
    emitSessionIdentity('user-role', 'CUSTOMER');
    const version = getSessionIdentityVersion();
    const listener = vi.fn();
    const stop = subscribeSessionIdentity(listener);

    emitSessionIdentity('user-role', 'COURIER');
    expect(listener).toHaveBeenCalledTimes(1);
    expect(getSessionIdentityVersion()).toBe(version);
    expect(getSessionRole()).toBe('COURIER');

    stop();
  });

  it('never keeps a role for a guest', () => {
    emitSessionIdentity('user-guest', 'MERCHANT');
    emitSessionIdentity(null);
    expect(getSessionRole()).toBeNull();
  });
});
