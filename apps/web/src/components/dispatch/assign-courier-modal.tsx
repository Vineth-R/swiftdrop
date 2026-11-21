'use client';

import { useEffect, useState } from 'react';

import type { ApiError } from '../../lib/api';
import {
  assignCourier,
  listCouriers,
  type AvailableCourier,
} from '../../lib/dispatch-api';
import { Button } from '../ui/button';
import { Modal } from '../ui/modal';
import { Select } from '../ui/select';
import { Textarea } from '../ui/textarea';
import { useToast } from '../ui/toast';
import { DispatchAlert } from './dispatch-alerts';
import { asApiError } from './use-paged-list';

type ParcelSummary = {
  id: string;
  trackingNumber: string;
  currentCourierId?: string | null;
};

type AssignCourierModalProps = {
  isOpen: boolean;
  onClose: () => void;
  parcel: ParcelSummary | null;
  onAssigned: () => void;
};

export function AssignCourierModal({
  isOpen,
  onClose,
  parcel,
  onAssigned,
}: AssignCourierModalProps) {
  const { show } = useToast();
  const [couriers, setCouriers] = useState<AvailableCourier[]>([]);
  const [isLoadingCouriers, setIsLoadingCouriers] = useState(false);
  const [couriersLoadError, setCouriersLoadError] = useState<ApiError | null>(
    null,
  );

  const [courierId, setCourierId] = useState('');
  const [reason, setReason] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [actionError, setActionError] = useState<ApiError | null>(null);
  const [fieldError, setFieldError] = useState<string | null>(null);

  // Load available couriers when the modal opens
  useEffect(() => {
    if (!isOpen) return;

    const controller = new AbortController();
    setIsLoadingCouriers(true);
    setCouriersLoadError(null);
    setCourierId(parcel?.currentCourierId ?? '');
    setReason('');
    setActionError(null);
    setFieldError(null);

    listCouriers({ isAvailable: true, pageSize: 100 }, controller.signal)
      .then(({ data }) => {
        setCouriers(data);
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) {
          setCouriersLoadError(asApiError(err));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoadingCouriers(false);
        }
      });

    return () => controller.abort();
  }, [isOpen, parcel]);

  if (!parcel) return null;

  const isReassignment = Boolean(parcel.currentCourierId);
  const title = isReassignment ? 'Reassign courier' : 'Assign courier';

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (!courierId) {
      setFieldError('Please select a courier');
      return;
    }

    if (!parcel) return;

    setIsSubmitting(true);
    setActionError(null);
    setFieldError(null);

    try {
      await assignCourier(parcel.id, {
        courierId,
        reason: reason.trim() || undefined,
      });

      show({
        title: isReassignment
          ? `Parcel ${parcel.trackingNumber} reassigned.`
          : `Parcel ${parcel.trackingNumber} assigned successfully.`,
      });
      onAssigned();
      onClose();
    } catch (err: unknown) {
      // Retain entered form state so dispatcher can adjust courier or retry
      setActionError(asApiError(err));
    } finally {
      setIsSubmitting(false);
    }
  }

  const courierOptions = couriers.map((courier) => ({
    value: courier.id,
    label: courier.vehicleType
      ? `${courier.fullName} (${courier.vehicleType})`
      : courier.fullName,
  }));

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={title}
      description={`Assign courier to parcel ${parcel.trackingNumber}.`}
      isDismissible={!isSubmitting}
      size="md"
      actions={
        <div className="flex w-full justify-end gap-3">
          <Button
            type="button"
            variant="secondary"
            onClick={onClose}
            isDisabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            form="assign-courier-form"
            isLoading={isSubmitting}
            isDisabled={isLoadingCouriers || couriers.length === 0}
          >
            {isReassignment ? 'Reassign' : 'Assign'}
          </Button>
        </div>
      }
    >
      <form
        id="assign-courier-form"
        onSubmit={handleSubmit}
        className="flex flex-col gap-4"
      >
        {couriersLoadError && (
          <DispatchAlert error={couriersLoadError} />
        )}

        {actionError && (
          <DispatchAlert error={actionError} />
        )}

        <Select
          id="courier-select"
          label="Courier"
          placeholder={
            isLoadingCouriers
              ? 'Loading available couriers...'
              : couriers.length === 0
                ? 'No couriers currently available'
                : 'Select an available courier'
          }
          options={courierOptions}
          value={courierId}
          disabled={isLoadingCouriers || isSubmitting || couriers.length === 0}
          error={fieldError ?? undefined}
          isRequired
          onChange={(event) => {
            setCourierId(event.target.value);
            setFieldError(null);
            setActionError(null);
          }}
          data-autofocus
        />

        <Textarea
          id="assign-reason"
          label="Reason (optional)"
          placeholder="e.g. Urgent morning dispatch requested or reassigned due to load"
          maxLength={255}
          showCount
          rows={3}
          value={reason}
          disabled={isSubmitting}
          onChange={(event) => setReason(event.target.value)}
        />
      </form>
    </Modal>
  );
}
