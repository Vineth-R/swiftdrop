'use client';

import { useEffect, useState, type FormEvent } from 'react';

import type { ApiError } from '../../lib/api';
import {
  createPricingRule,
  updatePricingRule,
  type PricingRule,
  type Zone,
} from '../../lib/pricing-api';
import { useForm, type FormRules } from '../../lib/use-form';
import { validateAmount } from '../../lib/validation';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { Select } from '../ui/select';
import { FailureAlert, fieldErrors } from './failure-alert';
import { asApiError } from './use-paged-list';

type Values = {
  originZoneId: string;
  destinationZoneId: string;
  baseFee: string;
};

const FORM_ID = 'rule-form';
const fields = ['originZoneId', 'destinationZoneId', 'baseFee'] as const;

const rules: FormRules<Values> = {
  originZoneId: (v) => (v.originZoneId ? undefined : 'Choose a zone.'),
  destinationZoneId: (v) =>
    v.destinationZoneId ? undefined : 'Choose a zone.',
  baseFee: (v) => validateAmount(v.baseFee, { isZeroAllowed: false }),
};

/** Add a rule, or change its base fee. The API does not let its zones change. */
export function RuleFormModal({
  isOpen,
  rule,
  route,
  zones,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  /** The rule being edited, or null to add one. */
  rule: PricingRule | null;
  /** The edited rule's two zones in words. */
  route: string;
  /** Active zones, the only ones a new rule can use. */
  zones: Zone[];
  onClose: () => void;
  onSaved: (rule: PricingRule) => void;
}) {
  const form = useForm<Values>(
    { originZoneId: '', destinationZoneId: '', baseFee: '' },
    rules,
  );
  const { setValue, resetValidation, setServerErrors } = form;
  const [isSaving, setIsSaving] = useState(false);
  const [problem, setProblem] = useState<ApiError | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    setValue('originZoneId', rule?.originZoneId ?? '');
    setValue('destinationZoneId', rule?.destinationZoneId ?? '');
    setValue('baseFee', rule ? rule.baseFee.toFixed(2) : '');
    setServerErrors({});
    resetValidation();
    setProblem(null);
  }, [isOpen, rule, setValue, setServerErrors, resetValidation]);

  const baseFee = Number(form.values.baseFee);
  const isDirty = rule === null || baseFee !== rule.baseFee;
  const options = zones.map((zone) => ({
    value: zone.id,
    label: zone.name,
  }));

  function focusFirstBad(found: Partial<Record<keyof Values, string>>) {
    const first = fields.find((field) => found[field]);
    if (first) document.getElementById(`rule-${first}`)?.focus();
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
        rule
          ? await updatePricingRule(rule.id, { baseFee })
          : await createPricingRule({
              originZoneId: form.values.originZoneId,
              destinationZoneId: form.values.destinationZoneId,
              baseFee,
            }),
      );
    } catch (reason) {
      const error = asApiError(reason);
      const mapped =
        error.code === 'RULE_EXISTS' && rule === null
          ? {
              destinationZoneId:
                'This route already has an active price. Edit that price instead.',
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
      title={rule ? `Edit ${route}` : 'Add a rule'}
      description={
        rule
          ? 'Editing a rule never changes the fee stored on existing parcels.'
          : 'Set the base fee for a pair of active zones.'
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
            {isSaving ? 'Saving' : rule ? 'Save rule' : 'Add rule'}
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
        {rule === null && (
          <>
            <Select
              id="rule-originZoneId"
              label="From"
              isRequired
              controlSize="lg"
              placeholder="Choose a zone"
              options={options}
              data-autofocus
              value={form.values.originZoneId}
              error={form.errors.originZoneId}
              onChange={(event) =>
                form.setValue('originZoneId', event.target.value)
              }
              onBlur={() => form.blur('originZoneId')}
            />
            <Select
              id="rule-destinationZoneId"
              label="To"
              isRequired
              controlSize="lg"
              placeholder="Choose a zone"
              options={options}
              value={form.values.destinationZoneId}
              error={form.errors.destinationZoneId}
              onChange={(event) =>
                form.setValue('destinationZoneId', event.target.value)
              }
              onBlur={() => form.blur('destinationZoneId')}
            />
          </>
        )}
        <TextField
          id="rule-baseFee"
          label="Base fee"
          prefix="LKR"
          isMono
          isRequired
          controlSize="lg"
          inputMode="decimal"
          autoComplete="off"
          data-autofocus={rule ? true : undefined}
          value={form.values.baseFee}
          error={form.errors.baseFee}
          onChange={(event) => form.setValue('baseFee', event.target.value)}
          onBlur={() => form.blur('baseFee')}
        />
      </form>
    </Modal>
  );
}
