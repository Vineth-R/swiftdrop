'use client';

import {
  useCallback,
  useEffect,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react';

import { ApiError, type ApiMeta } from '../../lib/api';

type Pagination = NonNullable<ApiMeta['pagination']>;

export function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

/**
 * One page of an admin list, loaded as the users screen loads its list. Pass
 * `load` through useCallback: a new `load` means the filters changed.
 */
export function usePagedList<Row>(
  load: (
    page: number,
    signal: AbortSignal,
  ) => Promise<{ data: Row[]; meta: ApiMeta }>,
) {
  const [page, setPage] = useState(1);
  const [reload, setReload] = useState(0);
  const [rows, setRows] = useState<Row[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  // It stays while the next page loads, so the paging buttons do not vanish
  // under the person's focus.
  const [pagination, setPagination] = useState<Pagination | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setRows(null);
    setError(null);
    load(page, controller.signal)
      .then(({ data, meta }) => {
        const next = meta.pagination ?? null;
        setPagination(next);
        // The pages can shrink while someone is on a later one.
        const lastPage = Math.max(next?.totalPages ?? 1, 1);
        if (page > lastPage) {
          setPage(lastPage);
          return;
        }
        setRows(data);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(asApiError(reason));
      });
    return () => controller.abort();
  }, [load, page, reload]);

  const refresh = useCallback(() => setReload((value) => value + 1), []);

  return {
    rows,
    setRows: setRows as Dispatch<SetStateAction<Row[] | null>>,
    error,
    page,
    setPage,
    pagination,
    refresh,
    isLoading: rows === null && error === null,
  };
}
