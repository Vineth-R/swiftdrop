'use client';

import { Mail } from 'lucide-react';
import { useEffect, useState, type FormEvent, type ReactNode } from 'react';

import { ApiError } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import {
  approveMerchant,
  getAdminMerchant,
  rejectMerchant,
  suspendMerchant,
  type Merchant,
} from '../../lib/merchant-api';
import { getZones, type Zone } from '../../lib/pricing-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { ConfirmationDialog } from '../ui/confirmation-dialog';
import { KeyValueList } from '../ui/key-value-list';
import { Modal } from '../ui/modal';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';
import { Textarea } from '../ui/textarea';
import { useToast } from '../ui/toast';
import { EmailCompositionDialog } from './email-composition-dialog';

type State =
  | { status: 'loading' }
  | { status: 'ready'; merchant: Merchant }
  | { status: 'error'; error: ApiError };

type Notice = { tone: 'success' | 'warning'; text: string } | null;
type View = 'details' | 'reject' | 'suspend';

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t border-line pt-5 first:border-t-0 first:pt-0">
      <h3 className="text-h4">{title}</h3>
      {children}
    </section>
  );
}

function actionMessage(error: ApiError) {
  switch (error.code) {
    case 'INVALID_MERCHANT_STATE':
      return 'That status change is not allowed from the merchant’s current state. Reload and try again.';
    case 'MERCHANT_ACTIVATION_FAILED':
      return 'Activation failed. The merchant remains inactive. Retry safely.';
    case 'NOT_FOUND':
      return 'This merchant no longer exists. Close the dialog and reload the list.';
    case 'VALIDATION_FAILED':
      return 'Check the reason and try again.';
    case 'NETWORK':
      return 'We could not reach SwiftDrop. Check your connection and try again.';
    default:
      return 'The decision could not be saved. Try again.';
  }
}

function syncNotice(kind: 'approve' | 'reject' | 'suspend', sync: string) {
  if (sync === 'PENDING') {
    return {
      tone: 'warning' as const,
      text: 'The decision was saved, but Identity has not applied it yet, so the account may still be signed in. Use Retry sync.',
    };
  }
  if (kind === 'approve') {
    return {
      tone: 'success' as const,
      text: 'Merchant approved and activated.',
    };
  }
  return {
    tone: 'success' as const,
    text:
      kind === 'reject'
        ? 'Merchant rejected.'
        : 'Merchant suspended and sessions invalidated.',
  };
}

type MerchantDetailsModalProps = {
  merchantId: string | null;
  onClose: () => void;
  onChanged: (merchant: Merchant) => void;
};

/**
 * The full merchant application: business and account facts, the current
 * lifecycle state, and the actions the backend allows from that state.
 */
export function MerchantDetailsModal({
  merchantId,
  onClose,
  onChanged,
}: MerchantDetailsModalProps) {
  const { show } = useToast();
  const [state, setState] = useState<State>({ status: 'loading' });
  const [reload, setReload] = useState(0);
  const [zones, setZones] = useState<Zone[]>([]);
  const [view, setView] = useState<View>('details');
  const [reason, setReason] = useState('');
  const [reasonError, setReasonError] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const [confirmApprove, setConfirmApprove] = useState(false);
  const [emailOpen, setEmailOpen] = useState(false);

  useEffect(() => {
    if (!merchantId) return;
    setView('details');
    setReason('');
    setReasonError('');
    setActionError(null);
    setNotice(null);
    setConfirmApprove(false);
    setEmailOpen(false);
  }, [merchantId]);

  useEffect(() => {
    if (!merchantId) return;
    const controller = new AbortController();
    setState({ status: 'loading' });
    getAdminMerchant(merchantId, controller.signal)
      .then((merchant) => setState({ status: 'ready', merchant }))
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ status: 'error', error: asApiError(error) });
        }
      });
    return () => controller.abort();
  }, [merchantId, reload]);

  useEffect(() => {
    if (!merchantId) return;
    const controller = new AbortController();
    getZones(controller.signal)
      .then((list) => {
        if (!controller.signal.aborted) setZones(list);
      })
      .catch(() => {
        if (!controller.signal.aborted) setZones([]);
      });
    return () => controller.abort();
  }, [merchantId]);

  const merchant = state.status === 'ready' ? state.merchant : null;
  const zoneName = (id?: string | null) =>
    (id && zones.find((zone) => zone.id === id)?.name) || id || null;

  function close() {
    if (busy) return;
    setView('details');
    setConfirmApprove(false);
    setEmailOpen(false);
    onClose();
  }

  function applyMerchant(updated: Merchant) {
    setState({ status: 'ready', merchant: updated });
    onChanged(updated);
  }

  async function decide(kind: 'approve' | 'reject' | 'suspend') {
    if (!merchant || busy) return;
    if (kind !== 'approve' && !reason.trim()) {
      setReasonError('Enter a reason. The merchant sees it in the email.');
      return;
    }
    setBusy(true);
    setActionError(null);
    try {
      const result =
        kind === 'approve'
          ? await approveMerchant(merchant.id)
          : kind === 'reject'
            ? await rejectMerchant(merchant.id, reason)
            : await suspendMerchant(merchant.id, reason);
      const updated: Merchant = {
        ...merchant,
        status: result.status,
        identitySync: result.identitySync === 'PENDING' ? 'PENDING' : undefined,
        approvedAt: result.approvedAt ?? merchant.approvedAt,
        rejectionReason:
          kind === 'approve'
            ? undefined
            : kind === 'reject'
              ? reason.trim()
              : merchant.rejectionReason,
        suspensionReason:
          kind === 'suspend'
            ? reason.trim()
            : kind === 'approve'
              ? undefined
              : merchant.suspensionReason,
      };
      applyMerchant(updated);
      setNotice(syncNotice(kind, result.identitySync));
      show({
        title:
          kind === 'approve'
            ? `${merchant.businessName} is approved.`
            : kind === 'reject'
              ? `${merchant.businessName} is rejected.`
              : `${merchant.businessName} is suspended.`,
      });
      setView('details');
      setConfirmApprove(false);
      setReason('');
      setReasonError('');
      setReload((value) => value + 1);
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setBusy(false);
    }
  }

  async function retrySync() {
    if (!merchant || busy) return;
    setBusy(true);
    setActionError(null);
    try {
      const kind = merchant.status === 'REJECTED' ? 'reject' : 'suspend';
      const result =
        kind === 'reject'
          ? await rejectMerchant(merchant.id)
          : await suspendMerchant(merchant.id, merchant.suspensionReason);
      applyMerchant({
        ...merchant,
        identitySync: result.identitySync === 'PENDING' ? 'PENDING' : undefined,
      });
      setNotice(
        result.identitySync === 'PENDING'
          ? {
              tone: 'warning',
              text: 'Identity could not be reached again. The account may still be signed in. Try again in a moment.',
            }
          : {
              tone: 'success',
              text: 'Identity applied the decision. The merchant is signed out everywhere.',
            },
      );
      setReload((value) => value + 1);
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setBusy(false);
    }
  }

  const canApprove =
    merchant?.status === 'PENDING' ||
    merchant?.status === 'SUSPENDED' ||
    merchant?.status === 'REJECTED';
  const canReject = merchant?.status === 'PENDING';
  const canSuspend = merchant?.status === 'APPROVED';

  const title =
    view === 'reject'
      ? 'Reject application'
      : view === 'suspend'
        ? 'Suspend merchant'
        : (merchant?.businessName ?? 'Merchant details');
  const description =
    view === 'reject'
      ? 'The merchant stays inactive. Explain what needs to change; the reason is shown in the email and on sign-in.'
      : view === 'suspend'
        ? 'Business operations stop immediately and existing sessions are invalidated.'
        : 'Business and account information, with the actions available in the current state.';

  return (
    <>
      <Modal
        isOpen={merchantId !== null}
        onClose={close}
        title={title}
        description={description}
        size="xl"
        isDismissible={!busy}
        actions={
          view === 'reject' || view === 'suspend' ? (
            <>
              <Button
                variant="secondary"
                isDisabled={busy}
                onClick={() => {
                  setView('details');
                  setReason('');
                  setReasonError('');
                  setActionError(null);
                }}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                form="merchant-reason-form"
                variant="danger"
                isLoading={busy}
              >
                {view === 'reject' ? 'Reject' : 'Suspend'}
              </Button>
            </>
          ) : (
            <>
              <Button variant="secondary" onClick={close} isDisabled={busy}>
                Close
              </Button>
              {merchant ? (
                <>
                  <Button
                    variant="secondary"
                    isDisabled={busy}
                    onClick={() => setEmailOpen(true)}
                  >
                    <Mail size={16} strokeWidth={1.5} aria-hidden="true" />
                    Compose email
                  </Button>
                  {canApprove ? (
                    <Button
                      isDisabled={busy}
                      onClick={() => {
                        setActionError(null);
                        setConfirmApprove(true);
                      }}
                    >
                      {merchant.status === 'PENDING' ? 'Approve' : 'Reactivate'}
                    </Button>
                  ) : null}
                  {canReject ? (
                    <Button
                      variant="danger"
                      isDisabled={busy}
                      onClick={() => {
                        setActionError(null);
                        setView('reject');
                      }}
                    >
                      Reject
                    </Button>
                  ) : null}
                  {canSuspend ? (
                    <Button
                      variant="danger"
                      isDisabled={busy}
                      onClick={() => {
                        setActionError(null);
                        setView('suspend');
                      }}
                    >
                      Suspend
                    </Button>
                  ) : null}
                </>
              ) : null}
            </>
          )
        }
      >
        {state.status === 'loading' ? (
          <SkeletonGroup label="Loading merchant details">
            <div className="flex flex-col gap-4">
              <Skeleton className="h-6 w-1/3" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
              <Skeleton className="h-4 w-2/3" />
            </div>
          </SkeletonGroup>
        ) : state.status === 'error' ? (
          <Alert
            tone={state.error.code === 'FORBIDDEN' ? 'warning' : 'danger'}
            title={
              state.error.code === 'FORBIDDEN'
                ? 'You do not have access to this'
                : state.error.code === 'NOT_FOUND'
                  ? 'This merchant no longer exists'
                  : 'Could not load this merchant'
            }
            actions={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setReload((value) => value + 1)}
              >
                Try again
              </Button>
            }
          >
            {actionMessage(state.error)}
          </Alert>
        ) : merchant && (view === 'reject' || view === 'suspend') ? (
          <form
            id="merchant-reason-form"
            noValidate
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault();
              void decide(view);
            }}
            className="flex flex-col gap-4"
          >
            {actionError ? (
              <Alert tone="danger" title="Could not complete the decision">
                {actionMessage(actionError)}
              </Alert>
            ) : null}
            <Textarea
              id="merchant-decision-reason"
              label="Reason"
              isRequired
              maxLength={1000}
              showCount
              rows={5}
              value={reason}
              error={reasonError || undefined}
              onChange={(event) => {
                setReason(event.target.value);
                setReasonError('');
              }}
              helper="Required. The merchant sees this reason in the email and when they try to sign in."
              data-autofocus
            />
          </form>
        ) : merchant ? (
          <div className="flex flex-col gap-5">
            {notice ? <Alert tone={notice.tone} title={notice.text} /> : null}
            {actionError ? (
              <Alert tone="danger" title="Could not complete the decision">
                {actionMessage(actionError)}
              </Alert>
            ) : null}
            {merchant.identitySync === 'PENDING' ? (
              <Alert
                tone="warning"
                title="Sign-out pending at Identity"
                actions={
                  <Button
                    variant="secondary"
                    size="sm"
                    isLoading={busy}
                    onClick={() => void retrySync()}
                  >
                    Retry sync
                  </Button>
                }
              >
                The decision is saved, but Identity has not applied it yet, so
                the account may still hold a session. Use Retry sync.
              </Alert>
            ) : null}

            <Group title="Business information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Business name', value: merchant.businessName },
                  {
                    label: 'Registration number',
                    value: merchant.registrationNumber,
                    isMono: true,
                  },
                  {
                    label: 'Business email',
                    value: merchant.contactEmail,
                    isMono: true,
                  },
                  {
                    label: 'Business phone',
                    value: merchant.contactPhone,
                    isMono: true,
                  },
                  {
                    label: 'Business address',
                    value: merchant.businessAddress,
                  },
                ]}
              />
            </Group>

            <Group title="Account information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Merchant ID', value: merchant.id, isMono: true },
                  {
                    label: 'Owner user ID',
                    value: merchant.ownerUserId,
                    isMono: true,
                  },
                  {
                    label: 'Submitted',
                    value: merchant.createdAt
                      ? formatDateTime(merchant.createdAt)
                      : null,
                    isMono: true,
                  },
                  {
                    label: 'Approved',
                    value: merchant.approvedAt
                      ? formatDateTime(merchant.approvedAt)
                      : null,
                    isMono: true,
                  },
                  {
                    label: 'Approved by',
                    value: merchant.approvedBy,
                    isMono: true,
                  },
                  {
                    label: 'Last updated',
                    value: merchant.updatedAt
                      ? formatDateTime(merchant.updatedAt)
                      : null,
                    isMono: true,
                  },
                ]}
              />
            </Group>

            <Group title="Operational information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Default pickup', value: merchant.pickupAddress },
                  {
                    label: 'Pickup zone',
                    value: zoneName(merchant.pickupZoneId),
                  },
                  {
                    label: 'Account status',
                    value: <StatusChip status={merchant.status} />,
                  },
                ]}
              />
            </Group>

            <Group title="Administrative information">
              <KeyValueList
                layout="inline"
                items={[
                  {
                    label: 'Rejection reason',
                    value: merchant.rejectionReason ?? null,
                  },
                  {
                    label: 'Suspension reason',
                    value: merchant.suspensionReason ?? null,
                  },
                ]}
              />
            </Group>
          </div>
        ) : null}
      </Modal>

      <ConfirmationDialog
        isOpen={confirmApprove}
        onClose={() => setConfirmApprove(false)}
        title={
          merchant ? `Approve ${merchant.businessName}?` : 'Approve merchant?'
        }
        description="The account is activated only after Identity applies the lifecycle transition."
        confirmLabel="Approve"
        tone="primary"
        isLoading={busy}
        onConfirm={() => void decide('approve')}
        error={actionError ? actionMessage(actionError) : undefined}
      />

      <EmailCompositionDialog
        merchant={emailOpen ? merchant : null}
        onClose={() => setEmailOpen(false)}
        onSent={() => setNotice({ tone: 'success', text: 'Email sent.' })}
      />
    </>
  );
}
