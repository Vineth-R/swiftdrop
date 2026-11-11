'use client';

import { useState } from 'react';

import { SessionExpiredDialog } from '../../components/auth/session-expired-dialog';
import { Button } from '../../components/ui/button';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
  type DataTableSort,
} from '../../components/ui/data-table';
import { EmptyState } from '../../components/ui/empty-state';
import { Modal } from '../../components/ui/modal';
import { Select } from '../../components/ui/select';
import { StatusChip, type ChipStatus } from '../../components/ui/status-chip';
import { Textarea } from '../../components/ui/textarea';
import { Timeline } from '../../components/ui/timeline';
import { useToast } from '../../components/ui/toast';
import { parcelStatuses } from '../../lib/tokens';
import { Panel } from './components-section';

type ParcelRow = {
  id: string;
  trackingNumber: string;
  city: string;
  status: ChipStatus;
  fee: string;
};

const parcels: ParcelRow[] = [
  {
    id: '1',
    trackingNumber: 'SDK7M2Q9XP4',
    city: 'Kandy',
    status: 'IN_TRANSIT',
    fee: '550.00',
  },
  {
    id: '2',
    trackingNumber: 'SDK3H8L1ZT6',
    city: 'Galle',
    status: 'DELIVERED',
    fee: '700.00',
  },
  {
    id: '3',
    trackingNumber: 'SDN5R2C7WB1',
    city: 'Jaffna',
    status: 'FAILED',
    fee: '1450.00',
  },
  {
    id: '4',
    trackingNumber: 'SDK9D4F6QY2',
    city: 'Colombo',
    status: 'CREATED',
    fee: '450.00',
  },
];

const columns: DataTableColumn<ParcelRow>[] = [
  {
    key: 'trackingNumber',
    header: 'Tracking number',
    cell: (row) => row.trackingNumber,
    isMono: true,
    isSortable: true,
  },
  { key: 'city', header: 'Destination', cell: (row) => row.city },
  {
    key: 'status',
    header: 'Status',
    cell: (row) => <StatusChip status={row.status} />,
  },
  {
    key: 'fee',
    header: 'Fee (LKR)',
    cell: (row) => row.fee,
    isNumeric: true,
    isSortable: true,
  },
  {
    key: 'open',
    header: 'Open',
    isHeaderHidden: true,
    cell: (row) => (
      <Button variant="secondary" size="sm">
        View {row.trackingNumber}
      </Button>
    ),
  },
];

function sortRows(rows: ParcelRow[], sort: DataTableSort): ParcelRow[] {
  const key = sort.key as 'trackingNumber' | 'fee';
  const sorted = [...rows].sort((a, b) =>
    key === 'fee'
      ? Number(a.fee) - Number(b.fee)
      : a.trackingNumber.localeCompare(b.trackingNumber),
  );
  return sort.direction === 'ascending' ? sorted : sorted.reverse();
}

const merchantStatuses: ChipStatus[] = [
  'PENDING',
  'APPROVED',
  'REJECTED',
  'SUSPENDED',
];
const accountStatuses: ChipStatus[] = ['ACTIVE', 'DEACTIVATED'];

const zones = [
  { value: 'cmb', label: 'Colombo' },
  { value: 'kdy', label: 'Kandy' },
  { value: 'gal', label: 'Galle' },
  { value: 'jaf', label: 'Jaffna', isDisabled: true },
];

type DialogName = 'confirm' | 'reason' | 'busy' | null;

/** The kit pieces for forms, dialogs, lists and feedback, each with its states. */
export function KitSection() {
  const { show } = useToast();
  const [dialog, setDialog] = useState<DialogName>(null);
  const [reason, setReason] = useState('');
  const [note, setNote] = useState('Left at the gate with the guard.');
  const [zone, setZone] = useState('');
  const [sort, setSort] = useState<DataTableSort>({
    key: 'trackingNumber',
    direction: 'ascending',
  });
  const [page, setPage] = useState(2);

  return (
    <section aria-labelledby="kit" className="flex flex-col gap-8">
      <h2 id="kit" className="font-display text-h2">
        Forms, dialogs, lists and feedback
      </h2>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Textarea">
          <Textarea
            id="demo-note"
            label="Delivery note"
            helper="Shown to the courier."
            maxLength={200}
            showCount
            value={note}
            onChange={(event) => setNote(event.target.value)}
          />
          <Textarea
            id="demo-note-error"
            label="Reason"
            isRequired
            error="Enter a reason of at least 5 characters."
            defaultValue=""
          />
          <Textarea
            id="demo-note-disabled"
            label="Internal remarks"
            disabled
            defaultValue="Only dispatchers can edit this."
          />
        </Panel>
        <Panel title="Select">
          <Select
            id="demo-zone"
            label="Pickup zone"
            placeholder="Choose a zone"
            helper="Where couriers collect your parcels."
            isRequired
            options={zones}
            value={zone}
            onChange={(event) => setZone(event.target.value)}
          />
          <Select
            id="demo-zone-error"
            label="Destination zone"
            placeholder="Choose a zone"
            error="Choose a destination zone."
            options={zones}
            defaultValue=""
          />
          <Select
            id="demo-zone-disabled"
            label="Home zone"
            disabled
            options={zones}
            defaultValue="kdy"
          />
        </Panel>
      </div>

      <Panel title="Status chips">
        <div className="flex flex-wrap items-center gap-3">
          {parcelStatuses.map((status) => (
            <StatusChip key={status} status={status} />
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {merchantStatuses.map((status) => (
            <StatusChip key={status} status={status} />
          ))}
          {accountStatuses.map((status) => (
            <StatusChip key={status} status={status} />
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <StatusChip status="DELIVERED" size="sm" />
          <StatusChip status="DELIVERED" isCompact />
          <span className="text-body-sm text-text-muted">
            Small, and compact for a very tight cell: the name stays for screen
            readers.
          </span>
        </div>
      </Panel>

      <Panel title="Data table (cards below 768px)">
        <DataTable
          caption="Parcels"
          columns={columns}
          rows={sortRows(parcels, sort)}
          getRowId={(row) => row.id}
          sort={sort}
          onSortChange={setSort}
        />
        <TablePagination
          page={page}
          totalPages={5}
          totalItems={42}
          onPageChange={setPage}
        />
      </Panel>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Data table, loading">
          <DataTable
            caption="Merchants"
            columns={columns}
            rows={[]}
            getRowId={(row) => row.id}
            isLoading
          />
        </Panel>
        <Panel title="Data table, empty">
          <DataTable
            caption="Merchants"
            columns={columns}
            rows={[]}
            getRowId={(row) => row.id}
            empty={
              <EmptyState title="No merchants waiting">
                New applications appear here for you to approve.
              </EmptyState>
            }
          />
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Modal">
          <div className="flex flex-wrap gap-3">
            <Button variant="secondary" onClick={() => setDialog('confirm')}>
              Open confirmation
            </Button>
            <Button variant="secondary" onClick={() => setDialog('reason')}>
              Open reason dialog
            </Button>
            <Button variant="secondary" onClick={() => setDialog('busy')}>
              Open session-expired dialog
            </Button>
          </div>
          <span className="text-body-sm text-text-muted">
            Escape, the close button and a click outside all close it, except
            the session-expired dialog, which must be answered.
          </span>
        </Panel>
        <Panel title="Toast">
          <div className="flex flex-wrap gap-3">
            <Button
              variant="secondary"
              onClick={() => show({ title: 'Details saved.', duration: 3000 })}
            >
              Show success
            </Button>
            <Button
              variant="secondary"
              onClick={() =>
                show({
                  tone: 'info',
                  title: 'Pricing updated.',
                  description: 'New parcels use the new rate.',
                  duration: 3000,
                })
              }
            >
              Show info
            </Button>
            <Button
              variant="secondary"
              onClick={() =>
                show({
                  tone: 'danger',
                  title: 'We could not save the details.',
                  description: 'Check your connection and try again.',
                  duration: 0,
                })
              }
            >
              Show error (stays)
            </Button>
          </div>
          <span className="text-body-sm text-text-muted">
            Five seconds by default, three here. Hover or focus pauses it. At
            most three show at once.
          </span>
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Timeline, tracking">
          <Timeline
            label="Tracking history"
            items={[
              {
                id: 'created',
                title: 'CREATED',
                at: '2026-11-03T03:10:00Z',
                detail: 'Colombo',
              },
              {
                id: 'picked',
                title: 'PICKED UP',
                at: '2026-11-03T05:40:00Z',
                detail: 'Colombo',
              },
              {
                id: 'transit',
                title: 'IN TRANSIT',
                at: '2026-11-03T09:25:00Z',
                detail: 'Kandy hub',
                isCurrent: true,
              },
              { id: 'out', title: 'OUT FOR DELIVERY', isUpcoming: true },
              { id: 'delivered', title: 'DELIVERED', isUpcoming: true },
            ]}
          />
        </Panel>
        <Panel title="Timeline, approval history">
          <Timeline
            label="Merchant approval history"
            items={[
              {
                id: 'applied',
                title: 'Application received',
                at: '2026-11-02T08:12:00Z',
              },
              {
                id: 'approved',
                title: 'Approved by an administrator',
                at: '2026-11-03T04:30:00Z',
                isCurrent: true,
              },
            ]}
          />
        </Panel>
      </div>

      <Modal
        isOpen={dialog === 'confirm'}
        onClose={() => setDialog(null)}
        title="Suspend ShopZone Lanka?"
        description="Its API keys stop working until you approve the merchant again."
        actions={
          <>
            <Button
              variant="secondary"
              data-autofocus
              onClick={() => setDialog(null)}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                setDialog(null);
                show({ title: 'Merchant suspended.', duration: 3000 });
              }}
            >
              Suspend
            </Button>
          </>
        }
      />
      <Modal
        isOpen={dialog === 'reason'}
        onClose={() => {
          setDialog(null);
          setReason('');
        }}
        title="Reject this application"
        description="The merchant sees your reason."
        actions={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setDialog(null);
                setReason('');
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              isDisabled={reason.trim().length < 5}
              onClick={() => {
                setDialog(null);
                setReason('');
                show({ title: 'Application rejected.', duration: 3000 });
              }}
            >
              Reject
            </Button>
          </>
        }
      >
        <Textarea
          id="demo-reject-reason"
          label="Reason"
          isRequired
          data-autofocus
          maxLength={200}
          showCount
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </Modal>
      <SessionExpiredDialog
        isOpen={dialog === 'busy'}
        onLogIn={() => setDialog(null)}
      />
    </section>
  );
}
