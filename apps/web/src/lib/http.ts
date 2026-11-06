import { ApiError } from './api-errors';
import { API_BASE } from './constants';

export type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  signal?: AbortSignal;
};

/**
 * The single HTTP entry point. Same-origin credentials let the browser attach
 * the HttpOnly session cookies; no token is ever read or stored here.
 */
export async function send(
  path: string,
  options: RequestOptions,
): Promise<Response> {
  try {
    return await fetch(`${API_BASE}${path}`, {
      method: options.method ?? 'GET',
      headers: {
        Accept: 'application/json',
        ...(options.body === undefined
          ? {}
          : { 'Content-Type': 'application/json' }),
      },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: 'same-origin',
      cache: 'no-store',
      signal: options.signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError')
      throw error;
    throw new ApiError({ code: 'NETWORK', status: 0 });
  }
}
