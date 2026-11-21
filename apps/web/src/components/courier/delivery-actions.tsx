'use client';

import { Ban, CircleCheck, MapPin, PackageCheck, Truck } from 'lucide-react';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import {
  confirmCourierDelivery,
  confirmCourierPickup,
  updateCourierDeliveryStatus,
  type CourierDeliveryDetail,
} from '../../lib/courier-api';
import { formatDateTime } from '../../lib/format';
import { useCountdown } from '../../lib/use-countdown';
import { useSession } from '../../lib/session';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import { TextField } from '../ui/field';
import { KeyValueList } from '../ui/key-value-list';
import { Modal } from '../ui/modal';
import { Textarea } from '../ui/textarea';
import {
  validateDelivery,
  validateFailure,
  validatePickup,
  type WorkflowErrors,
} from './delivery-upload';
import { proofPhotoError } from './proof-upload';
import { PhotoProofInput } from './photo-proof-input';

type Confirmed = (message: string) => void;

type RequestState = {
  saving: boolean;
  error: ApiError | null;
  blocked: boolean;
  remaining: number;
  reset: () => void;
  run: (
    action: (signal: AbortSignal) => Promise<unknown>,
    message?: string,
  ) => Promise<boolean>;
};

function useWorkflowRequest(onConfirmed: Confirmed): RequestState {
  const { recheck } = useSession();
  const request = useRef<AbortController | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const remaining = useCountdown(
    error?.code === 'RATE_LIMITED' ? (error.retryAfter ?? null) : null,
  );
  useEffect(() => () => request.current?.abort(), []);

  const blocked =
    saving ||
    remaining > 0 ||
    error?.code === 'FORBIDDEN' ||
    error?.code === 'NOT_FOUND' ||
    error?.code === 'UNAUTHENTICATED' ||
    error?.code === 'ACCOUNT_DEACTIVATED';

  async function run(
    action: (signal: AbortSignal) => Promise<unknown>,
    message = 'The delivery was updated.',
  ): Promise<boolean> {
    if (blocked || request.current) return false;
    const controller = new AbortController();
    request.current = controller;
    setSaving(true);
    setError(null);
    try {
      await action(controller.signal);
      if (controller.signal.aborted) return false;
      onConfirmed(message);
      return true;
    } catch (caught) {
      if (controller.signal.aborted) return false;
      const next =
        caught instanceof ApiError
          ? caught
          : new ApiError({ code: 'INTERNAL', status: 0 });
      if (next.code === 'ACCOUNT_DEACTIVATED') recheck();
      setError(next);
      return false;
    } finally {
      request.current = null;
      if (!controller.signal.aborted) setSaving(false);
    }
  }

  return {
    saving,
    error,
    blocked,
    remaining,
    reset: () => setError(null),
    run,
  };
}

function focusField(id: string) {
  document.getElementById(id)?.focus();
}

/** Only the next valid action for the current status is ever shown. */
export function DeliveryActionForStatus({
  delivery,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  delivery: CourierDeliveryDetail;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  switch (delivery.status) {
    case 'CREATED':
      return (
        <PickupAction
          parcelId={delivery.id}
          onConfirmed={onConfirmed}
          onRefresh={onRefresh}
          isRefreshing={isRefreshing}
        />
      );
    case 'PICKED_UP':
      return (
        <ContinueAction
          parcelId={delivery.id}
          onConfirmed={onConfirmed}
          onRefresh={onRefresh}
          isRefreshing={isRefreshing}
        />
      );
    case 'IN_TRANSIT':
      return (
        <OutForDeliveryAction
          parcelId={delivery.id}
          onConfirmed={onConfirmed}
          onRefresh={onRefresh}
          isRefreshing={isRefreshing}
        />
      );
    case 'OUT_FOR_DELIVERY':
      return (
        <DeliveryAction
          parcelId={delivery.id}
          onConfirmed={onConfirmed}
          onRefresh={onRefresh}
          isRefreshing={isRefreshing}
        />
      );
    default:
      return null;
  }
}

function PickupAction({
  parcelId,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  parcelId: string;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  const [photo, setPhoto] = useState<File | null>(null);
  const [pickerKey, setPickerKey] = useState(0);
  const [errors, setErrors] = useState<WorkflowErrors>({});
  const write = useWorkflowRequest(onConfirmed);
  const blocked = write.blocked || isRefreshing;

  function choosePhoto(file: File | null) {
    setErrors((current) => ({ ...current, photo: undefined }));
    if (!file) {
      setPhoto(null);
      return;
    }
    const problem = proofPhotoError(file);
    if (problem) {
      setPickerKey((key) => key + 1);
      setPhoto(null);
      setErrors((current) => ({ ...current, photo: problem }));
      return;
    }
    setPhoto(file);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    const next = validatePickup(photo);
    setErrors(next);
    if (next.photo) {
      focusField('courier-pickup-photo');
      return;
    }
    if (!photo) return;
    void write.run(
      (signal) => confirmCourierPickup(parcelId, { photo }, signal),
      'Pickup confirmed. The parcel is now in transit.',
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-body text-text-muted">
          Confirm that you have collected the parcel. The photo is required and
          the parcel moves to in transit automatically.
        </p>
        {write.error && (
          <WorkflowError
            error={write.error}
            remaining={write.remaining}
            isRefreshing={isRefreshing}
            onRefresh={() => {
              write.reset();
              onRefresh();
            }}
          />
        )}
        <form noValidate onSubmit={submit} className="flex flex-col gap-4">
          <PhotoProofInput
            key={pickerKey}
            id="courier-pickup-photo"
            label="Pickup proof photo"
            helper="JPEG or PNG, up to 5 MiB. Take a clear photo of the parcel."
            photo={photo}
            error={errors.photo}
            disabled={blocked}
            onChange={choosePhoto}
          />
          {write.saving && (
            <p role="status" className="text-body-sm text-text-muted">
              Uploading the pickup photo and saving the status. Keep this page
              open.
            </p>
          )}
          <Button
            type="submit"
            size="lg"
            className="self-start"
            isLoading={write.saving}
            isDisabled={!write.saving && blocked}
            iconLeading={
              <PackageCheck size={20} strokeWidth={1.5} aria-hidden="true" />
            }
          >
            {write.saving ? 'Confirming pickup' : 'Confirm pickup'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function ContinueAction({
  parcelId,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  parcelId: string;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  const write = useWorkflowRequest(onConfirmed);
  const blocked = write.blocked || isRefreshing;
  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-body text-text-muted">
          Pickup was already recorded for this parcel. Continue without adding
          another photo.
        </p>
        {write.error && (
          <WorkflowError
            error={write.error}
            remaining={write.remaining}
            isRefreshing={isRefreshing}
            onRefresh={() => {
              write.reset();
              onRefresh();
            }}
          />
        )}
        <Button
          type="button"
          size="lg"
          className="self-start"
          isLoading={write.saving}
          isDisabled={!write.saving && blocked}
          iconLeading={<Truck size={20} strokeWidth={1.5} aria-hidden="true" />}
          onClick={() => {
            if (blocked) return;
            void write.run(
              (signal) =>
                updateCourierDeliveryStatus(
                  parcelId,
                  { status: 'IN_TRANSIT' },
                  signal,
                ),
              'The parcel is now in transit.',
            );
          }}
        >
          {write.saving ? 'Continuing delivery' : 'Continue to in transit'}
        </Button>
      </CardContent>
    </Card>
  );
}

function OutForDeliveryAction({
  parcelId,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  parcelId: string;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  const write = useWorkflowRequest(onConfirmed);
  const blocked = write.blocked || isRefreshing;
  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-body text-text-muted">
          Start the final leg when you leave for the drop-off address.
        </p>
        {write.error && (
          <WorkflowError
            error={write.error}
            remaining={write.remaining}
            isRefreshing={isRefreshing}
            onRefresh={() => {
              write.reset();
              onRefresh();
            }}
          />
        )}
        <Button
          type="button"
          size="lg"
          className="self-start"
          isLoading={write.saving}
          isDisabled={!write.saving && blocked}
          iconLeading={
            <MapPin size={20} strokeWidth={1.5} aria-hidden="true" />
          }
          onClick={() => {
            if (blocked) return;
            void write.run(
              (signal) =>
                updateCourierDeliveryStatus(
                  parcelId,
                  { status: 'OUT_FOR_DELIVERY' },
                  signal,
                ),
              'The parcel is now out for delivery.',
            );
          }}
        >
          {write.saving ? 'Marking out for delivery' : 'Mark out for delivery'}
        </Button>
      </CardContent>
    </Card>
  );
}

function DeliveryAction({
  parcelId,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  parcelId: string;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  const [photo, setPhoto] = useState<File | null>(null);
  const [pickerKey, setPickerKey] = useState(0);
  const [recipientName, setRecipientName] = useState('');
  const [note, setNote] = useState('');
  const [errors, setErrors] = useState<WorkflowErrors>({});
  const [isConfirming, setIsConfirming] = useState(false);
  const write = useWorkflowRequest(onConfirmed);
  const blocked = write.blocked || isRefreshing;

  function choosePhoto(file: File | null) {
    setErrors((current) => ({ ...current, photo: undefined }));
    if (!file) {
      setPhoto(null);
      return;
    }
    const problem = proofPhotoError(file);
    if (problem) {
      setPickerKey((key) => key + 1);
      setPhoto(null);
      setErrors((current) => ({ ...current, photo: problem }));
      return;
    }
    setPhoto(file);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    const next = validateDelivery({ photo, recipientName, note });
    setErrors(next);
    if (next.recipientName) {
      focusField('courier-delivery-recipient');
      return;
    }
    if (next.photo) {
      focusField('courier-delivery-photo');
      return;
    }
    if (next.note) {
      focusField('courier-delivery-note');
      return;
    }
    setIsConfirming(true);
  }

  async function confirm() {
    if (!photo || blocked) return;
    setIsConfirming(false);
    await write.run(
      (signal) =>
        confirmCourierDelivery(
          parcelId,
          { photo, recipientName: recipientName.trim(), note: note.trim() },
          signal,
        ),
      'Delivery confirmed. The parcel is now delivered.',
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <p className="text-body text-text-muted">
          Record who received the parcel and add a handover photo. Both are
          required.
        </p>
        {write.error && (
          <WorkflowError
            error={write.error}
            remaining={write.remaining}
            isRefreshing={isRefreshing}
            onRefresh={() => {
              write.reset();
              onRefresh();
            }}
          />
        )}
        <form noValidate onSubmit={submit} className="flex flex-col gap-4">
          <TextField
            id="courier-delivery-recipient"
            label="Recipient name"
            helper="The person who received the parcel. Up to 150 characters."
            controlSize="lg"
            isRequired
            name="recipientName"
            value={recipientName}
            error={errors.recipientName}
            disabled={blocked}
            onChange={(event) => {
              setRecipientName(event.target.value);
              setErrors((current) => ({
                ...current,
                recipientName: undefined,
              }));
            }}
          />
          <PhotoProofInput
            key={pickerKey}
            id="courier-delivery-photo"
            label="Delivery proof photo"
            helper="JPEG or PNG, up to 5 MiB. Take a clear photo of the handover."
            photo={photo}
            error={errors.photo}
            disabled={blocked}
            onChange={choosePhoto}
          />
          <Textarea
            id="courier-delivery-note"
            label="Delivery note"
            helper="Optional, up to 500 characters."
            controlSize="lg"
            name="note"
            maxLength={500}
            showCount
            value={note}
            error={errors.note}
            disabled={blocked}
            onChange={(event) => {
              setNote(event.target.value);
              setErrors((current) => ({ ...current, note: undefined }));
            }}
          />
          {write.saving && (
            <p role="status" className="text-body-sm text-text-muted">
              Uploading the delivery photo and saving the status. Keep this page
              open.
            </p>
          )}
          <Button
            type="submit"
            size="lg"
            className="self-start"
            isLoading={write.saving}
            isDisabled={!write.saving && blocked}
            iconLeading={
              <CircleCheck size={20} strokeWidth={1.5} aria-hidden="true" />
            }
          >
            {write.saving ? 'Confirming delivery' : 'Confirm delivery'}
          </Button>
        </form>
      </CardContent>
      <Modal
        isOpen={isConfirming}
        onClose={() => setIsConfirming(false)}
        title="Confirm delivery?"
        description="The parcel will be marked delivered. This cannot be undone."
        actions={
          <>
            <Button
              size="lg"
              variant="secondary"
              onClick={() => setIsConfirming(false)}
            >
              Cancel
            </Button>
            <Button size="lg" data-autofocus onClick={() => void confirm()}>
              Confirm delivery
            </Button>
          </>
        }
      >
        <KeyValueList
          items={[
            { label: 'Recipient', value: recipientName.trim() },
            { label: 'Photo', value: photo?.name ?? 'No photo selected' },
            { label: 'Note', value: note.trim() },
          ]}
        />
      </Modal>
    </Card>
  );
}

/** The existing FAILED transition, kept as the secondary path. */
export function FailureAction({
  parcelId,
  onConfirmed,
  onRefresh,
  isRefreshing,
}: {
  parcelId: string;
  onConfirmed: Confirmed;
  onRefresh: () => void;
  isRefreshing: boolean;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [reasonError, setReasonError] = useState<string>();
  const write = useWorkflowRequest(onConfirmed);
  const blocked = write.blocked || isRefreshing;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked) return;
    const problem = validateFailure(reason);
    setReasonError(problem);
    if (problem) {
      focusField('courier-failure-reason');
      return;
    }
    void (async () => {
      const saved = await write.run(
        (signal) =>
          updateCourierDeliveryStatus(
            parcelId,
            { status: 'FAILED', failureReason: reason.trim() },
            signal,
          ),
        'Delivery failure recorded.',
      );
      if (saved) setIsOpen(false);
    })();
  }

  return (
    <>
      <Button
        type="button"
        size="lg"
        variant="secondary"
        isDisabled={blocked}
        iconLeading={<Ban size={20} strokeWidth={1.5} aria-hidden="true" />}
        onClick={() => {
          write.reset();
          setIsOpen(true);
        }}
      >
        Report delivery failure
      </Button>
      <Modal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        title="Report a failed delivery"
        description="Say why the parcel could not be delivered. This closes the delivery."
        isDismissible={!write.saving}
        actions={
          <>
            <Button
              size="lg"
              variant="secondary"
              onClick={() => setIsOpen(false)}
            >
              Cancel
            </Button>
            <Button
              size="lg"
              variant="danger"
              type="submit"
              form="courier-failure-form"
              isLoading={write.saving}
              isDisabled={!write.saving && blocked}
            >
              {write.saving ? 'Reporting failure' : 'Report failure'}
            </Button>
          </>
        }
      >
        <form
          id="courier-failure-form"
          noValidate
          onSubmit={submit}
          className="flex flex-col gap-4"
        >
          {write.error && (
            <WorkflowError
              error={write.error}
              remaining={write.remaining}
              isRefreshing={isRefreshing}
              onRefresh={() => {
                write.reset();
                onRefresh();
              }}
            />
          )}
          <Textarea
            id="courier-failure-reason"
            label="Failure reason"
            helper={`${Array.from(reason.trim()).length}/255 characters; at least 5 required.`}
            controlSize="lg"
            isRequired
            name="failureReason"
            maxLength={255}
            value={reason}
            error={reasonError}
            disabled={blocked}
            onChange={(event) => {
              setReason(event.target.value);
              setReasonError(undefined);
            }}
          />
        </form>
      </Modal>
    </>
  );
}

export function TerminalState({
  delivery,
}: {
  delivery: CourierDeliveryDetail;
}) {
  const delivered = [...delivery.history]
    .reverse()
    .find((entry) => entry.status === 'DELIVERED');
  const failed = [...delivery.history]
    .reverse()
    .find((entry) => entry.status === 'FAILED');
  const deliveryProof = delivery.proofs.find(
    (proof) => proof.kind === 'DELIVERY',
  );

  if (delivery.status === 'DELIVERED') {
    return (
      <Card>
        <CardContent className="flex flex-col gap-4">
          <Alert tone="success" title="Delivered">
            The parcel was handed over successfully. No further status action is
            available.
          </Alert>
          <KeyValueList
            items={[
              {
                label: 'Received by',
                value: deliveryProof?.recipientName,
              },
              {
                label: 'Delivered at',
                value: delivered?.changedAt
                  ? formatDateTime(delivered.changedAt)
                  : undefined,
              },
              { label: 'Note', value: deliveryProof?.note },
            ]}
          />
        </CardContent>
      </Card>
    );
  }

  if (delivery.status === 'FAILED') {
    return (
      <Card>
        <CardContent className="flex flex-col gap-4">
          <Alert tone="danger" title="Delivery failed">
            {failed?.failureReason ??
              'No failure reason was recorded for this delivery.'}
          </Alert>
          <KeyValueList
            items={[
              {
                label: 'Failed at',
                value: failed?.changedAt
                  ? formatDateTime(failed.changedAt)
                  : undefined,
              },
              { label: 'Note', value: failed?.note },
            ]}
          />
        </CardContent>
      </Card>
    );
  }

  return null;
}

function WorkflowError({
  error,
  remaining,
  isRefreshing,
  onRefresh,
}: {
  error: ApiError;
  remaining: number;
  isRefreshing: boolean;
  onRefresh: () => void;
}) {
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  if (error.code === 'RATE_LIMITED')
    return <RateLimitedAlert remaining={remaining} />;
  if (error.code === 'FORBIDDEN')
    return (
      <Alert tone="danger" title="You no longer have access to this delivery">
        Return to your deliveries and contact your dispatcher if the assignment
        needs attention.
      </Alert>
    );
  if (error.code === 'NOT_FOUND')
    return (
      <Alert tone="danger" title="Delivery not found">
        Return to your deliveries and choose an assigned parcel.
      </Alert>
    );
  if (error.code === 'CONFLICT')
    return (
      <Alert
        tone="warning"
        title="This delivery changed before you saved it"
        actions={
          <Button
            size="lg"
            variant="secondary"
            isLoading={isRefreshing}
            onClick={onRefresh}
          >
            Refresh delivery
          </Button>
        }
      >
        Your input is still here. Refresh the delivery and review its current
        status before trying again.
      </Alert>
    );
  if (error.code === 'VALIDATION_FAILED')
    return (
      <Alert tone="danger" title="Check the delivery details">
        Review the photo, recipient name and note, then try again. Your input is
        still here.
      </Alert>
    );
  if (error.isServerFault)
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.requestId}
      />
    );
  return (
    <Alert tone="danger" title="The delivery could not be updated">
      Your input is still here. Review it and try again.
    </Alert>
  );
}
