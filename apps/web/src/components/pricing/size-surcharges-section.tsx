'use client';

import { useEffect, useState, type FormEvent } from 'react';

import type { ApiError } from '../../lib/api';
import { formatLkr } from '../../lib/format';
import {
  listSizeSurcharges,
  setSizeSurcharge,
  type SizeCategory,
  type SizeSurcharge,
} from '../../lib/pricing-api';
import { useForm, type FormRules } from '../../lib/use-form';
import { validateAmount } from '../../lib/validation';
import { Section } from '../layout/section';
import { Button } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { useToast } from '../ui/toast';
import { FailureAlert, fieldErrors } from './failure-alert';
import { asApiError } from './use-paged-list';

const FORM_ID = 'surcharge-form';

const sizeNames: Record<SizeCategory, string> = {
  SMALL: 'Small',
  MEDIUM: 'Medium',
  LARGE: 'Large',
  OVERSIZED: 'Oversized',
};

const rules: FormRules<{ surcharge: string }> = {
  surcharge: (v) => validateAmount(v.surcharge, { isZeroAllowed: true }),
};

/** One surcharge per size. The four sizes are fixed, so there is nothing to add. */
export function SizeSurchargesSection() {
  const { show } = useToast();
  const [rows, setRows] = useState<SizeSurcharge[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [reload, setReload] = useState(0);
  const [editing, setEditing] = useState<SizeSurcharge | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [problem, setProblem] = useState<ApiError | null>(null);
  const form = useForm({ surcharge: '' }, rules);

  useEffect(() => {
    const controller = new AbortController();
    setRows(null);
    setError(null);
    listSizeSurcharges(controller.signal)
      .then(setRows)
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(asApiError(reason));
      });
    return () => controller.abort();
  }, [reload]);

  function startEditing(row: SizeSurcharge) {
    setEditing(row);
    form.setServerErrors({});
    form.setValue('surcharge', row.surcharge.toFixed(2));
    form.resetValidation();
    setProblem(null);
    setIsOpen(true);
  }

  const surcharge = Number(form.values.surcharge);
  const isDirty = editing !== null && surcharge !== editing.surcharge;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSaving || !isDirty || !editing) return;
    setProblem(null);
    if (form.validate().surcharge) {
      document.getElementById('surcharge-amount')?.focus();
      return;
    }

    setIsSaving(true);
    try {
      const saved = await setSizeSurcharge(editing.sizeCategory, surcharge);
      setRows(
        (current) =>
          current?.map((row) =>
            row.sizeCategory === saved.sizeCategory ? saved : row,
          ) ?? current,
      );
      setIsOpen(false);
      show({ title: `${sizeNames[saved.sizeCategory]} surcharge saved.` });
    } catch (reason) {
      const failure = asApiError(reason);
      const mapped = fieldErrors(failure, ['surcharge'] as const);
      if (mapped.surcharge) {
        form.setServerErrors(mapped);
        document.getElementById('surcharge-amount')?.focus();
      } else {
        setProblem(failure);
      }
    } finally {
      setIsSaving(false);
    }
  }

  const columns: DataTableColumn<SizeSurcharge>[] = [
    {
      key: 'size',
      header: 'Size',
      cell: (row) => sizeNames[row.sizeCategory],
    },
    {
      key: 'surcharge',
      header: 'Surcharge',
      isNumeric: true,
      isMono: true,
      cell: (row) => formatLkr(row.surcharge),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (row) => (
        <Button
          variant="secondary"
          size="lg"
          aria-label={`Edit the ${sizeNames[row.sizeCategory].toLowerCase()} surcharge`}
          onClick={() => startEditing(row)}
        >
          Edit
        </Button>
      ),
    },
  ];

  return (
    <Section
      title="Size surcharges"
      description="The charge added for a parcel's size."
    >
      {error ? (
        <FailureAlert
          error={error}
          onRetry={() => setReload((value) => value + 1)}
        />
      ) : (
        <DataTable
          caption="Size surcharges"
          columns={columns}
          rows={rows ?? []}
          getRowId={(row) => row.sizeCategory}
          isLoading={rows === null}
          empty={
            <EmptyState title="No size surcharges yet">
              The four sizes appear here once the pricing data is set up.
            </EmptyState>
          }
        />
      )}

      <Modal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        isDismissible={!isSaving}
        title={
          editing
            ? `Edit the ${sizeNames[editing.sizeCategory].toLowerCase()} surcharge`
            : ''
        }
        description="Editing a surcharge never changes the fee stored on existing parcels."
        actions={
          <>
            <Button
              variant="secondary"
              size="lg"
              isDisabled={isSaving}
              onClick={() => setIsOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              form={FORM_ID}
              size="lg"
              isLoading={isSaving}
              isDisabled={!isDirty}
            >
              {isSaving ? 'Saving' : 'Save surcharge'}
            </Button>
          </>
        }
      >
        <form
          id={FORM_ID}
          noValidate
          onSubmit={submit}
          className="flex flex-col gap-4"
        >
          {problem && <FailureAlert error={problem} />}
          <TextField
            id="surcharge-amount"
            label="Surcharge"
            prefix="LKR"
            isMono
            isRequired
            controlSize="lg"
            inputMode="decimal"
            autoComplete="off"
            data-autofocus
            value={form.values.surcharge}
            error={form.errors.surcharge}
            onChange={(event) => form.setValue('surcharge', event.target.value)}
            onBlur={() => form.blur('surcharge')}
          />
        </form>
      </Modal>
    </Section>
  );
}
