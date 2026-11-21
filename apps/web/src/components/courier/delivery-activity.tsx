'use client';

import {
  getCourierProofPhoto,
  type CourierDeliveryDetail,
} from '../../lib/courier-api';
import { formatDateTime } from '../../lib/format';
import type { ParcelStatus } from '../../lib/tokens';
import { Section } from '../layout/section';
import { ProofPhotoButton } from '../proof-photo-button';
import { Card, CardContent } from '../ui/card';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import { Timeline, type TimelineItem } from '../ui/timeline';

const statusTitles: Record<ParcelStatus, string> = {
  CREATED: 'Created',
  PICKED_UP: 'Picked up',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
};

const actorLabels: Record<string, string> = {
  COURIER: 'Recorded by courier',
  DISPATCHER: 'Recorded by dispatch',
  ADMIN: 'Recorded by an administrator',
  CUSTOMER: 'Recorded by the customer',
  MERCHANT: 'Recorded by the merchant',
};

function historyOf(delivery: CourierDeliveryDetail): TimelineItem[] {
  return delivery.history.map((entry, index) => {
    const actor = entry.actorRole ? actorLabels[entry.actorRole] : undefined;
    const detail = [entry.failureReason, entry.note, actor]
      .filter((value): value is string => Boolean(value?.trim()))
      .join(' · ');
    return {
      id: `${entry.changedAt}-${entry.status}-${index}`,
      title: statusTitles[entry.status],
      at: entry.changedAt,
      detail: detail || undefined,
      isCurrent: index === delivery.history.length - 1,
    };
  });
}

/**
 * Status history and proof photos in one place. Every proof is fetched through
 * the authorized per-proof photo route, so no storage path or permanent URL is
 * ever exposed.
 */
export function DeliveryActivity({
  delivery,
}: {
  delivery: CourierDeliveryDetail;
}) {
  return (
    <Section
      title="Activity"
      description="Status history, courier actions and proof photos."
    >
      {delivery.proofs.length > 0 && (
        <div className="grid min-w-0 gap-4 md:grid-cols-2">
          {delivery.proofs.map((proof) => (
            <Card key={proof.id}>
              <CardContent className="flex flex-col gap-4">
                <h3 className="text-h4 font-semibold">
                  {proof.kind === 'PICKUP' ? 'Pickup proof' : 'Delivery proof'}
                </h3>
                <KeyValueList
                  items={[
                    {
                      label: 'Recorded',
                      value: formatDateTime(proof.createdAt),
                    },
                    { label: 'Recipient', value: proof.recipientName },
                    { label: 'Note', value: proof.note },
                  ]}
                />
                {proof.hasPhoto && (
                  <ProofPhotoButton
                    load={(signal) =>
                      getCourierProofPhoto(delivery.id, signal, proof.id)
                    }
                  />
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {delivery.history.length === 0 ? (
        <EmptyState title="No status history yet">
          Updates will appear here after the parcel changes status.
        </EmptyState>
      ) : (
        <Timeline
          label="Status history"
          className="break-words"
          items={historyOf(delivery)}
        />
      )}
    </Section>
  );
}
