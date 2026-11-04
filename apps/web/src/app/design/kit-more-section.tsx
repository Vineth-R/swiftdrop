'use client';

import { Copy, EllipsisVertical, Trash2, X } from 'lucide-react';
import { useState } from 'react';

import { ButtonLink } from '../../components/ui/button';
import { Checkbox, CheckboxGroup } from '../../components/ui/checkbox';
import { TextField } from '../../components/ui/field';
import { FileField } from '../../components/ui/file-field';
import { IconButton } from '../../components/ui/icon-button';
import { KeyValueList } from '../../components/ui/key-value-list';
import { RadioGroup } from '../../components/ui/radio-group';
import { WaybillStub } from '../../components/ui/waybill-stub';
import { Panel } from './components-section';

const events = [
  { value: 'created', label: 'Created' },
  { value: 'picked-up', label: 'Picked up' },
  { value: 'in-transit', label: 'In transit' },
  { value: 'out', label: 'Out for delivery' },
  { value: 'delivered', label: 'Delivered' },
  { value: 'failed', label: 'Failed', description: 'Includes the reason.' },
];

const sizes = [
  {
    value: 'small',
    label: 'Small',
    description: 'Up to 5 kg, fits a shoebox.',
  },
  {
    value: 'medium',
    label: 'Medium',
    description: 'Up to 10 kg, fits a carton.',
  },
  {
    value: 'large',
    label: 'Large',
    description: 'Over 10 kg. Two hands.',
    isDisabled: true,
  },
];

const waybillRows = [
  { label: 'Last update', value: '3 Nov 2026, 14:55' },
  { label: 'Recipient', value: 'Nimali Perera', isPrivate: true },
  { label: 'Fee', value: 'LKR 550.00', isMono: true, isPrivate: true },
];

/** Choices, files, small controls and the parcel and public-page pieces. */
export function KitMoreSection() {
  const [subscribed, setSubscribed] = useState(['created', 'delivered']);
  const [size, setSize] = useState('');
  const [speed, setSpeed] = useState('standard');
  const [photo, setPhoto] = useState<string | null>(null);

  return (
    <section aria-labelledby="kit-more" className="flex flex-col gap-8">
      <h2 id="kit-more" className="font-display text-h2">
        Choices, files, details and the public page
      </h2>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="Checkbox">
          <Checkbox id="demo-terms" label="I accept the terms" isRequired />
          <Checkbox
            id="demo-email"
            label="Email me when a parcel is delivered"
            description="One email per status change."
            defaultChecked
          />
          <Checkbox
            id="demo-terms-error"
            label="I accept the terms"
            isRequired
            error="Accept the terms to continue."
          />
          <Checkbox
            id="demo-notify"
            label="Notify the courier"
            disabled
            defaultChecked
          />
          <Checkbox
            id="demo-all"
            label="All events"
            isIndeterminate={
              subscribed.length > 0 && subscribed.length < events.length
            }
            checked={subscribed.length === events.length}
            onChange={(event) =>
              setSubscribed(
                event.target.checked ? events.map((item) => item.value) : [],
              )
            }
          />
        </Panel>
        <Panel title="Checkbox group">
          <CheckboxGroup
            name="demo-events"
            legend="Send a webhook when a parcel is"
            helper="Choose at least one."
            options={events}
            values={subscribed}
            onChange={setSubscribed}
          />
          <CheckboxGroup
            name="demo-events-error"
            legend="Notify me by"
            isRequired
            error="Choose at least one way."
            options={[
              { value: 'email', label: 'Email' },
              { value: 'sms', label: 'Text message' },
            ]}
            values={[]}
            onChange={() => undefined}
          />
        </Panel>
        <Panel title="Radio group">
          <RadioGroup
            name="demo-size"
            legend="Parcel size"
            isRequired
            helper="The size sets the fee."
            options={sizes}
            value={size}
            onChange={setSize}
          />
          <RadioGroup
            name="demo-speed"
            legend="Delivery speed"
            orientation="horizontal"
            options={[
              { value: 'standard', label: 'Standard' },
              { value: 'express', label: 'Express' },
            ]}
            value={speed}
            onChange={setSpeed}
          />
          <RadioGroup
            name="demo-size-error"
            legend="Pickup window"
            error="Choose a pickup window."
            options={[
              { value: 'am', label: 'Morning' },
              { value: 'pm', label: 'Afternoon' },
            ]}
            value=""
            onChange={() => undefined}
          />
        </Panel>
        <Panel title="Date and time (TextField)">
          <TextField
            id="demo-date"
            label="Pickup date"
            type="date"
            min="2026-11-04"
            defaultValue="2026-11-05"
            helper="Parcels are collected between 09:00 and 17:00."
          />
          <TextField
            id="demo-time"
            label="Earliest pickup time"
            type="time"
            defaultValue="09:30"
          />
          <TextField
            id="demo-datetime"
            label="Delivery window starts"
            type="datetime-local"
            isRequired
            error="Choose a date and time."
          />
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        <Panel title="File field">
          <FileField
            id="demo-photo"
            label="Proof photo"
            helper={
              photo
                ? `Chosen: ${photo}`
                : 'JPEG or PNG, up to 5 MB. On a phone this opens the camera.'
            }
            accept="image/jpeg,image/png"
            capture="environment"
            buttonLabel="Take or choose a photo"
            emptyLabel="No photo yet"
            controlSize="lg"
            onFileChange={(file) => setPhoto(file?.name ?? null)}
          />
          <FileField
            id="demo-photo-error"
            label="Proof photo"
            isRequired
            error="Add a photo of the delivered parcel."
            onFileChange={() => undefined}
          />
          <FileField
            id="demo-photo-disabled"
            label="Receipt scan"
            disabled
            onFileChange={() => undefined}
          />
        </Panel>
        <Panel title="Icon button">
          <div className="flex flex-wrap items-center gap-2">
            <IconButton
              icon={EllipsisVertical}
              label="Actions for SDK7M2Q9XP4"
            />
            <IconButton
              icon={Copy}
              label="Copy tracking number SDK7M2Q9XP4"
              variant="secondary"
            />
            <IconButton
              icon={Trash2}
              label="Revoke key sdk_live_4f2a"
              variant="danger"
            />
            <IconButton icon={X} label="Close notice" size="sm" />
            <IconButton icon={X} label="Close notice, large" size="lg" />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <IconButton icon={Copy} label="Copying" isLoading />
            <IconButton icon={Copy} label="Copy is not available" isDisabled />
          </div>
          <span className="text-body-sm text-text-muted">
            44px by default, 48px on courier screens, 8px between neighbours.
            The label is the tooltip and the name a screen reader says.
          </span>
        </Panel>
        <Panel title="Key value list">
          <KeyValueList
            items={[
              { label: 'Business name', value: 'ShopZone Lanka' },
              {
                label: 'Registration number',
                value: 'PV 123456',
                isMono: true,
              },
              { label: 'Contact phone', value: '' },
              { label: 'Pickup address', value: '88 Galle Road, Colombo 03' },
            ]}
          />
        </Panel>
        <Panel title="Key value list, inline from 768px">
          <KeyValueList
            layout="inline"
            items={[
              { label: 'Order', value: 'SDN5R2C7WB1', isMono: true },
              { label: 'Created', value: '3 Nov 2026, 08:40' },
              { label: 'Fee', value: 'LKR 1450.00', isMono: true },
            ]}
          />
        </Panel>
      </div>

      <Panel title="Waybill stub">
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <WaybillStub
            trackingNumber="SDK7M2Q9XP4"
            status="IN_TRANSIT"
            origin="Colombo"
            destination="Kandy"
            footRows={waybillRows}
            isPublic
          />
          <WaybillStub
            trackingNumber="SDN5R2C7WB1"
            status="DELIVERED"
            origin="Colombo"
            destination="Jaffna"
            footRows={waybillRows}
          />
          <div className="flex flex-col gap-3">
            <WaybillStub
              trackingNumber="SDK3H8L1ZT6"
              status="OUT_FOR_DELIVERY"
              origin="Colombo"
              destination="Galle"
              isCompact
            />
            <WaybillStub
              trackingNumber="SDK9D4F6QY2"
              status="FAILED"
              origin="Kandy"
              destination="Colombo"
              isCompact
            />
          </div>
          <WaybillStub
            trackingNumber=""
            status="CREATED"
            origin=""
            destination=""
            isLoading
          />
        </div>
      </Panel>

      <Panel title="Public layout">
        <p className="text-body text-text-muted">
          The frame for pages anyone can open, such as public tracking: a header
          with the logo, the theme switch and Log in, one reading column and no
          login check.
        </p>
        <div>
          <ButtonLink href="/track" variant="secondary">
            Open /track
          </ButtonLink>
        </div>
      </Panel>
    </section>
  );
}
