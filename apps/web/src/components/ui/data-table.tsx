import { ArrowDown, ArrowUp, ChevronsUpDown } from 'lucide-react';
import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';
import { Button } from './button';
import { EmptyState } from './empty-state';
import { Skeleton, SkeletonGroup } from './skeleton';

export type DataTableColumn<Row> = {
  /** Stable name of the column. It is also the sort key sent to your API. */
  key: string;
  header: string;
  /** What the cell shows for a row. */
  cell: (row: Row) => ReactNode;
  /** Amounts and counts: right aligned, mono, tabular (rule 03). */
  isNumeric?: boolean;
  /** Identifiers such as tracking numbers and IDs: mono. */
  isMono?: boolean;
  /** Lets the person sort by this column. */
  isSortable?: boolean;
  /** An actions column: the header stays for screen readers only. */
  isHeaderHidden?: boolean;
};

export type DataTableSort = {
  key: string;
  direction: 'ascending' | 'descending';
};

type DataTableProps<Row> = {
  /** Names the table for screen readers, such as "Merchants". */
  caption: string;
  columns: DataTableColumn<Row>[];
  rows: Row[];
  getRowId: (row: Row) => string;
  /** Shows grey rows instead of data while the first page loads. */
  isLoading?: boolean;
  /** Shown when there are no rows. Defaults to a plain "Nothing here yet". */
  empty?: ReactNode;
  sort?: DataTableSort;
  /** Called with the next sort when a sortable header is pressed. */
  onSortChange?: (sort: DataTableSort) => void;
  className?: string;
};

const loadingRows = 5;

/** What pressing a header does: first ascending, then flip. */
function nextSort(key: string, current?: DataTableSort): DataTableSort {
  if (current?.key === key) {
    return {
      key,
      direction: current.direction === 'ascending' ? 'descending' : 'ascending',
    };
  }
  return { key, direction: 'ascending' };
}

function cellLook<Row>(column: DataTableColumn<Row>) {
  return cn(
    column.isNumeric && 'text-right font-mono tabular',
    column.isMono && 'font-mono text-body-sm',
  );
}

/**
 * Rows and columns for lists such as merchants, users and parcels. From 768px
 * it is a real table in its own scrolling box; below that the same columns
 * become a list of cards (DataTableCardList), so the page never scrolls
 * sideways. Loading and empty are part of it; show an `Alert` above it for an
 * error. The table has hairlines and no shadow.
 */
export function DataTable<Row>({
  caption,
  columns,
  rows,
  getRowId,
  isLoading,
  empty,
  sort,
  onSortChange,
  className,
}: DataTableProps<Row>) {
  if (isLoading) {
    return (
      <SkeletonGroup
        label={`Loading ${caption.toLowerCase()}`}
        className={cn(
          'overflow-hidden rounded-card border border-line bg-surface',
          className,
        )}
      >
        {Array.from({ length: loadingRows }, (_, index) => (
          <div
            key={index}
            className="flex items-center gap-6 border-b border-line p-4 last:border-b-0"
          >
            {columns.map((column, position) => (
              <Skeleton
                key={column.key}
                className={cn(
                  'h-4',
                  position === 0 ? 'w-1/3' : 'hidden w-1/5 md:block',
                )}
              />
            ))}
          </div>
        ))}
      </SkeletonGroup>
    );
  }

  if (rows.length === 0) {
    return <>{empty ?? <EmptyState title="Nothing here yet" />}</>;
  }

  return (
    <div className={className}>
      <div className="hidden overflow-x-auto rounded-card border border-line bg-surface md:block">
        <table className="w-full border-collapse text-left text-body">
          <caption className="sr-only">{caption}</caption>
          <thead>
            <tr className="border-b border-line bg-surface-2">
              {columns.map((column) => {
                const isSorted = sort?.key === column.key;
                return (
                  <th
                    key={column.key}
                    scope="col"
                    aria-sort={isSorted ? sort.direction : undefined}
                    className={cn(
                      'px-4 py-3 text-overline uppercase text-text-muted',
                      column.isNumeric && 'text-right',
                    )}
                  >
                    {column.isHeaderHidden ? (
                      <span className="sr-only">{column.header}</span>
                    ) : column.isSortable && onSortChange ? (
                      <button
                        type="button"
                        onClick={() => onSortChange(nextSort(column.key, sort))}
                        className="inline-flex items-center gap-1.5 uppercase hover:text-text"
                      >
                        {column.header}
                        {isSorted ? (
                          sort.direction === 'ascending' ? (
                            <ArrowUp
                              size={14}
                              strokeWidth={1.5}
                              aria-hidden="true"
                            />
                          ) : (
                            <ArrowDown
                              size={14}
                              strokeWidth={1.5}
                              aria-hidden="true"
                            />
                          )
                        ) : (
                          <ChevronsUpDown
                            size={14}
                            strokeWidth={1.5}
                            aria-hidden="true"
                          />
                        )}
                      </button>
                    ) : (
                      column.header
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {rows.map((row) => (
              <tr key={getRowId(row)} className="hover:bg-surface-2">
                {columns.map((column) => (
                  <td
                    key={column.key}
                    className={cn('px-4 py-3 align-middle', cellLook(column))}
                  >
                    {column.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <DataTableCardList
        className="md:hidden"
        columns={columns}
        rows={rows}
        getRowId={getRowId}
      />
    </div>
  );
}

type DataTableCardListProps<Row> = Pick<
  DataTableProps<Row>,
  'columns' | 'rows' | 'getRowId' | 'className'
>;

/**
 * The phone layout of a DataTable: one card per row. The first column is the
 * card's title, the other columns are label and value pairs, and an actions
 * column sits at the bottom. DataTable uses it below 768px; use it directly
 * only for a list that is always cards.
 */
export function DataTableCardList<Row>({
  columns,
  rows,
  getRowId,
  className,
}: DataTableCardListProps<Row>) {
  const [title, ...rest] = columns;
  const details = rest.filter((column) => !column.isHeaderHidden);
  const actions = rest.filter((column) => column.isHeaderHidden);

  return (
    <ul className={cn('flex flex-col gap-3', className)}>
      {rows.map((row) => (
        <li
          key={getRowId(row)}
          className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4"
        >
          <div className={cn('text-body font-semibold', cellLook(title))}>
            {title.cell(row)}
          </div>
          <dl className="flex flex-col gap-2">
            {details.map((column) => (
              <div
                key={column.key}
                className="flex items-baseline justify-between gap-4"
              >
                <dt className="text-caption text-text-muted">
                  {column.header}
                </dt>
                <dd className={cn('text-body-sm', cellLook(column))}>
                  {column.cell(row)}
                </dd>
              </div>
            ))}
          </dl>
          {actions.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {actions.map((column) => (
                <div key={column.key}>{column.cell(row)}</div>
              ))}
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}

type TablePaginationProps = {
  /** The current page, counting from 1 as the API does. */
  page: number;
  totalPages: number;
  /** When known, shown as "42 results". */
  totalItems?: number;
  onPageChange: (page: number) => void;
  /** True while the next page loads, so a second click does nothing. */
  isLoading?: boolean;
};

/** Previous and next under a list, with where you are: "Page 2 of 5". */
export function TablePagination({
  page,
  totalPages,
  totalItems,
  onPageChange,
  isLoading,
}: TablePaginationProps) {
  if (totalPages <= 1) return null;

  return (
    <nav
      aria-label="Pagination"
      className="flex flex-wrap items-center justify-between gap-3"
    >
      <p className="text-body-sm text-text-muted">
        <span className="tabular">
          Page {page} of {totalPages}
        </span>
        {totalItems !== undefined && (
          <>
            {' · '}
            <span className="tabular">{totalItems}</span> results
          </>
        )}
      </p>
      <div className="flex gap-2">
        <Button
          variant="secondary"
          size="sm"
          isDisabled={isLoading || page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          Previous
        </Button>
        <Button
          variant="secondary"
          size="sm"
          isDisabled={isLoading || page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          Next
        </Button>
      </div>
    </nav>
  );
}
