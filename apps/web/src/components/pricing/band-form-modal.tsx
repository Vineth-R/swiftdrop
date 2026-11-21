'use client';

import { useEffect, useState, type FormEvent } from 'react';

import type { ApiError } from '../../lib/api';
import { formatKg } from '../../lib/format';
import {
  createWeightBand,
  updateWeightBand,
  type WeightBand,
} from '../../lib/pricing-api';
import { useForm, type FormRules } from '../../lib/use-form';
import { validateAmount, validateWeight } from '../../lib/validation';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { FailureAlert, fieldErrors } from './failure-alert';
import { asApiError } from './use-paged-list';

type Values = { minWeightKg: string; maxWeightKg: string; charge: string };

const FORM_ID = 'band-form';
const fields = ['minWeightKg', 'maxWeightKg', 'charge'] as const;

const rules: FormRules<Values> = {
  minWeightKg: (v) => validateWeight(v.minWeightKg),
  maxWeightKg: (v) => {
    const invalid = validateWeight(v.maxWeightKg);
    if (invalid) return invalid;
    return Number(v.maxWeightKg) > Number(v.minWeightKg)
      ? undefined
      : 'Enter a weight above the "over" weight.';
  },
  charge: (v) => validateAmount(v.charge, { isZeroAllowed: true }),
};

/**
 * The API only says that a band overlaps, not which one, so look for it in the
 * bands on screen. An overlap with a band on another page has no name to give.
 */
function overlapMessage(
  min: number,
  max: number,
  bands: WeightBand[],
  editedId?: string,
): string {
  const other = bands.find(
    (band) =>
      band.isActive &&
      band.id !== editedId &&
      min < band.maxWeightKg &&
      band.minWeightKg < max,
  );
  const which = other
    ? `${formatKg(other.minWeightKg)} to ${formatKg(other.maxWeightKg)}`
    : 'another active band';
  return `This band overlaps ${which}. Change the weights so they do not overlap.`;
}

/** Add a weight band, or edit one. */
export function BandFormModal({
  isOpen,
  band,
  loadedBands,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  /** The band being edited, or null to add one. */
  band: WeightBand | null;
  loadedBands: WeightBand[];
  onClose: () => void;
  onSaved: (band: WeightBand) => void;
}) {
  const form = useForm<Values>(
    { minWeightKg: '', maxWeightKg: '', charge: '' },
    rules,
  );
  const { setValue, resetValidation, setServerErrors } = form;
  const [isSaving, setIsSaving] = useState(false);
  const [problem, setProblem] = useState<ApiError | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    setValue('minWeightKg', band ? band.minWeightKg.toFixed(2) : '');
    setValue('maxWeightKg', band ? band.maxWeightKg.toFixed(2) : '');
    setValue('charge', band ? band.charge.toFixed(2) : '');
    setServerErrors({});
    resetValidation();
    setProblem(null);
  }, [isOpen, band, setValue, setServerErrors, resetValidation]);

  const minWeightKg = Number(form.values.minWeightKg);
  const maxWeightKg = Number(form.values.maxWeightKg);
  const charge = Number(form.values.charge);
  const changes = band
    ? {
        ...(minWeightKg !== band.minWeightKg ? { minWeightKg } : {}),
        ...(maxWeightKg !== band.maxWeightKg ? { maxWeightKg } : {}),
        ...(charge !== band.charge ? { charge } : {}),
      }
    : null;
  const isDirty = changes === null || Object.keys(changes).length > 0;

  function focusFirstBad(found: Partial<Record<keyof Values, string>>) {
    const first = fields.find((field) => found[field]);
    if (first) document.getElementById(`band-${first}`)?.focus();
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSaving || !isDirty) return;
    setProblem(null);
    const found = form.validate();
    if (Object.keys(found).length > 0) {
      focusFirstBad(found);
      return;
    }

    setIsSaving(true);
    try {
      onSaved(
        band
          ? await updateWeightBand(band.id, changes ?? {})
          : await createWeightBand({ minWeightKg, maxWeightKg, charge }),
      );
    } catch (reason) {
      const error = asApiError(reason);
      const mapped =
        error.code === 'BAND_OVERLAP'
          ? {
              minWeightKg: overlapMessage(
                minWeightKg,
                maxWeightKg,
                loadedBands,
                band?.id,
              ),
            }
          : fieldErrors(error, fields);
      if (Object.keys(mapped).length > 0) {
        form.setServerErrors(mapped);
        focusFirstBad(mapped);
      } else {
        setProblem(error);
      }
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      isDismissible={!isSaving}
      title={band ? 'Edit weight band' : 'Add a weight band'}
      description={
        band
          ? 'Editing a band never changes the fee stored on existing parcels.'
          : 'Active bands cannot overlap, so each weight has one charge.'
      }
      actions={
        <>
          <Button
            variant="secondary"
            size="lg"
            isDisabled={isSaving}
            onClick={onClose}
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
            {isSaving ? 'Saving' : band ? 'Save band' : 'Add band'}
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
          id="band-minWeightKg"
          label="Over"
          suffix="kg"
          isMono
          isRequired
          controlSize="lg"
          inputMode="decimal"
          autoComplete="off"
          helper="The band covers parcels over this weight."
          data-autofocus
          value={form.values.minWeightKg}
          error={form.errors.minWeightKg}
          onChange={(event) => form.setValue('minWeightKg', event.target.value)}
          onBlur={() => form.blur('minWeightKg')}
        />
        <TextField
          id="band-maxWeightKg"
          label="Up to"
          suffix="kg"
          isMono
          isRequired
          controlSize="lg"
          inputMode="decimal"
          autoComplete="off"
          helper="And up to this weight, including it."
          value={form.values.maxWeightKg}
          error={form.errors.maxWeightKg}
          onChange={(event) => form.setValue('maxWeightKg', event.target.value)}
          onBlur={() => form.blur('maxWeightKg')}
        />
        <TextField
          id="band-charge"
          label="Charge"
          prefix="LKR"
          isMono
          isRequired
          controlSize="lg"
          inputMode="decimal"
          autoComplete="off"
          value={form.values.charge}
          error={form.errors.charge}
          onChange={(event) => form.setValue('charge', event.target.value)}
          onBlur={() => form.blur('charge')}
        />
      </form>
    </Modal>
  );
}
