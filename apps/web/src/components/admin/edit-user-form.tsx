'use client';

import { Lock } from 'lucide-react';
import { useRouter } from 'next/navigation';
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react';

import {
  updateAdminUser,
  type AdminUserDetail,
  type EditableRole,
  type UserChange,
} from '../../lib/admin-users-api';
import { ApiError } from '../../lib/api';
import { classifyApiError, type ApiProblem } from '../../lib/api-problem';
import { getZones, type Zone } from '../../lib/pricing-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  serverFieldMessage,
  toInternational,
  toSriLankanE164,
  toSriLankanLocal,
  validateFullName,
  validateInternationalPhone,
  validatePhone,
} from '../../lib/validation';
import { ApiProblemAlerts } from '../auth/api-problem-alerts';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { Checkbox } from '../ui/checkbox';
import { ErrorSummary, type ErrorSummaryIssue } from '../ui/error-summary';
import { TextField } from '../ui/field';
import { RadioGroup } from '../ui/radio-group';
import { Select } from '../ui/select';
import { useToast } from '../ui/toast';

type Values = {
  fullName: string;
  phone: string;
  role: string;
  vehicleType: string;
  vehicleNumber: string;
  homeZoneId: string;
};
type Field = keyof Values;

/** The longest the API accepts (identity service, courier profile). */
const VEHICLE_TYPE_MAX = 30;
const VEHICLE_NUMBER_MAX = 20;

const roleOptions: {
  value: EditableRole;
  label: string;
  description: string;
}[] = [
  {
    value: 'CUSTOMER',
    label: 'Customer',
    description: 'Sends and tracks parcels.',
  },
  {
    value: 'COURIER',
    label: 'Courier',
    description: 'Picks up and delivers parcels.',
  },
  {
    value: 'DISPATCHER',
    label: 'Dispatcher',
    description: 'Assigns parcels to couriers and reads reports.',
  },
  {
    value: 'ADMIN',
    label: 'Administrator',
    description: 'Manages users, zones, pricing and merchants.',
  },
];

const labels: Record<Field, string> = {
  fullName: 'Full name',
  phone: 'Phone',
  role: 'Role',
  vehicleType: 'Vehicle type',
  vehicleNumber: 'Vehicle number',
  homeZoneId: 'Home zone',
};

const courierFields: Field[] = ['vehicleType', 'vehicleNumber', 'homeZoneId'];

/** The id of each field's first focusable control. */
const fieldId = (field: Field) =>
  field === 'role' ? 'edit-user-role-CUSTOMER' : `edit-user-${field}`;

/** The API's field names, mapped onto ours. */
const serverFields: Record<string, Field> = {
  fullName: 'fullName',
  phone: 'phone',
  role: 'role',
  courierProfile: 'role',
  'courierProfile.vehicleType': 'vehicleType',
  'courierProfile.vehicleNumber': 'vehicleNumber',
  'courierProfile.homeZoneId': 'homeZoneId',
};

/**
 * A Sri Lankan number (or none yet) is typed after a fixed +94. Any other
 * number is shown and edited whole, with its country code.
 */
function isLocalNumber(user: AdminUserDetail): boolean {
  return user.phone === '' || toSriLankanLocal(user.phone) !== undefined;
}

function valuesOf(user: AdminUserDetail): Values {
  const profile = user.courierProfile;
  return {
    fullName: user.fullName,
    phone: isLocalNumber(user)
      ? (toSriLankanLocal(user.phone) ?? '')
      : user.phone,
    role: user.role,
    vehicleType: profile?.vehicleType ?? '',
    vehicleNumber: profile?.vehicleNumber ?? '',
    homeZoneId: profile?.homeZoneId ?? '',
  };
}

/**
 * Only what the person actually changed, so the API never sees untouched
 * fields. A courier profile goes only for a courier, and always when the role
 * becomes COURIER, because the API requires one then.
 */
function changesOf(
  user: AdminUserDetail,
  values: Values,
  isAvailable: boolean,
): UserChange {
  const change: UserChange = {};
  const name = values.fullName.trim();
  if (name !== user.fullName) change.fullName = name;
  if (values.phone.trim() !== '') {
    const phone = isLocalNumber(user)
      ? toSriLankanE164(values.phone)
      : toInternational(values.phone);
    if (phone !== user.phone) change.phone = phone;
  }
  if (values.role !== user.role) change.role = values.role as EditableRole;

  if (values.role === 'COURIER') {
    const before = user.courierProfile;
    const profile: NonNullable<UserChange['courierProfile']> = {};
    const type = values.vehicleType.trim();
    if (type && type !== (before?.vehicleType ?? '')) {
      profile.vehicleType = type;
    }
    const number = values.vehicleNumber.trim();
    if (number && number !== (before?.vehicleNumber ?? '')) {
      profile.vehicleNumber = number;
    }
    if (values.homeZoneId && values.homeZoneId !== (before?.homeZoneId ?? '')) {
      profile.homeZoneId = values.homeZoneId;
    }
    if (isAvailable !== (before?.isAvailable ?? true)) {
      profile.isAvailable = isAvailable;
    }
    if (Object.keys(profile).length > 0 || user.role !== 'COURIER') {
      change.courierProfile = profile;
    }
  }
  return change;
}

type Zones =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ready'; zones: Zone[] }
  | { status: 'error' };

type Banner = ApiProblem | { kind: 'forbidden' } | { kind: 'not-found' };

/**
 * The edit form of /admin/users/{id}/edit. An administrator changes a user's
 * name, phone, role and, for a courier, vehicle details and availability.
 * The email and password are not editable through the API.
 */
export function EditUserForm({ user }: { user: AdminUserDetail }): ReactNode {
  const router = useRouter();
  const { session, recheck } = useSession();
  const { show } = useToast();

  const isSelf =
    session.status === 'authenticated' && session.user.id === user.id;
  const isMerchant = user.role === 'MERCHANT';
  const isLocal = isLocalNumber(user);
  const hadPhone = user.phone !== '';
  const before = user.courierProfile;

  // The rules depend on this account, so they are built once per account.
  const rules = useMemo<FormRules<Values>>(
    () => ({
      fullName: (v) => validateFullName(v.fullName),
      phone: (v) => {
        if (v.phone.trim() === '') {
          return hadPhone
            ? 'Enter a full number, for example 71 234 5678.'
            : undefined;
        }
        return isLocal
          ? validatePhone(v.phone)
          : validateInternationalPhone(v.phone);
      },
      role: () => undefined,
      vehicleType: (v) => {
        if (v.role !== 'COURIER') return undefined;
        const length = [...v.vehicleType.trim()].length;
        if (length > VEHICLE_TYPE_MAX) {
          return `Use ${VEHICLE_TYPE_MAX} characters or fewer.`;
        }
        return before?.vehicleType && length === 0
          ? 'This cannot be cleared. Enter a new value.'
          : undefined;
      },
      vehicleNumber: (v) => {
        if (v.role !== 'COURIER') return undefined;
        const length = [...v.vehicleNumber.trim()].length;
        if (length > VEHICLE_NUMBER_MAX) {
          return `Use ${VEHICLE_NUMBER_MAX} characters or fewer.`;
        }
        return before?.vehicleNumber && length === 0
          ? 'This cannot be cleared. Enter a new value.'
          : undefined;
      },
      homeZoneId: () => undefined,
    }),
    [hadPhone, isLocal, before?.vehicleType, before?.vehicleNumber],
  );

  const form = useForm<Values>(valuesOf(user), rules);
  const [isAvailable, setIsAvailable] = useState(before?.isAvailable ?? true);
  const [banner, setBanner] = useState<Banner | null>(null);
  const [failedSubmits, setFailedSubmits] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [zones, setZones] = useState<Zones>({ status: 'idle' });
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    banner && 'seconds' in banner ? banner.seconds : null,
  );
  const isWaiting = banner?.kind === 'rate-limited' && waiting > 0;
  const showCourierFields = form.values.role === 'COURIER';
  const change = changesOf(user, form.values, isAvailable);
  const isDirty = Object.keys(change).length > 0;

  useEffect(() => {
    if (banner) alertRef.current?.focus();
  }, [banner]);

  // The zone list is only needed for a courier. A failure here never blocks
  // the form: a zone is optional.
  useEffect(() => {
    if (!showCourierFields) return;
    const controller = new AbortController();
    setZones({ status: 'loading' });
    getZones(controller.signal)
      .then((list) => {
        if (!controller.signal.aborted)
          setZones({ status: 'ready', zones: list });
      })
      .catch(() => {
        if (!controller.signal.aborted) setZones({ status: 'error' });
      });
    return () => controller.abort();
  }, [showCourierFields]);

  const visibleFields = (Object.keys(labels) as Field[]).filter(
    (field) => showCourierFields || !courierFields.includes(field),
  );
  const issues: ErrorSummaryIssue[] = visibleFields
    .filter((field) => form.errors[field])
    .map((field) => ({
      fieldId: fieldId(field),
      label: labels[field],
      message: (form.errors[field] ?? '').replace(/^./, (c) => c.toLowerCase()),
    }));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    setBanner(null);

    const found = form.validate();
    if (visibleFields.some((field) => found[field])) {
      // The error summary takes focus and links to each field.
      setFailedSubmits((count) => count + 1);
      return;
    }
    if (!isDirty) return;

    setIsLoading(true);
    try {
      const updated = await updateAdminUser(user.id, change);
      if (isSelf) recheck();
      show({ title: `${updated.fullName} was updated.` });
      router.push('/admin/users');
    } catch (error) {
      setIsLoading(false);
      handleFailure(error);
    }
  }

  function handleFailure(error: unknown) {
    if (error instanceof ApiError) {
      if (error.code === 'NOT_FOUND') {
        setBanner({ kind: 'not-found' });
        return;
      }
      if (error.code === 'FORBIDDEN') {
        setBanner({ kind: 'forbidden' });
        return;
      }
      if (error.code === 'CONFLICT') {
        // The only conflict an edit can hit: an administrator changing their own role.
        form.setServerErrors({ role: 'You cannot change your own role.' });
        setFailedSubmits((count) => count + 1);
        return;
      }
      if (error.code === 'UNAUTHENTICATED') {
        // The session could not be renewed. The session gate is already
        // sending the person to the login page.
        return;
      }
      if (error.code === 'ACCOUNT_DEACTIVATED') {
        recheck();
        return;
      }
    }

    const classified = classifyApiError(error);
    if (classified.kind === 'validation') {
      const fields: Partial<Record<Field, string>> = {};
      for (const issue of classified.issues) {
        const field = serverFields[issue.field];
        const message = serverFieldMessage(issue.field);
        if (field && message) fields[field] = message;
      }
      if (Object.keys(fields).length > 0) {
        form.setServerErrors(fields);
        setFailedSubmits((count) => count + 1);
        return;
      }
    }
    setBanner(classified);
  }

  const currentZoneId = before?.homeZoneId ?? '';
  const zoneOptions =
    zones.status === 'ready'
      ? [
          ...zones.zones.map((zone) => ({ value: zone.id, label: zone.name })),
          // A zone that is no longer in the public list stays selectable as it is.
          ...(currentZoneId && !zones.zones.some((z) => z.id === currentZoneId)
            ? [{ value: currentZoneId, label: 'Current zone (not listed)' }]
            : []),
        ]
      : currentZoneId
        ? [{ value: currentZoneId, label: 'Current zone' }]
        : [];
  const zoneHelper =
    zones.status === 'error'
      ? 'We could not load the zones. You can save without changing it.'
      : currentZoneId
        ? 'A zone can be changed here but not removed.'
        : 'Optional. You can set it later.';

  return (
    <>
      {banner?.kind === 'forbidden' && (
        <Alert
          ref={alertRef}
          tone="warning"
          icon={Lock}
          title="You do not have access to this"
        >
          Only administrators can edit users. If you need access, ask an
          administrator.
        </Alert>
      )}
      {banner?.kind === 'not-found' && (
        <Alert
          ref={alertRef}
          tone="warning"
          title="This account no longer exists"
          actions={
            <ButtonLink href="/admin/users" variant="secondary" size="sm">
              Back to users
            </ButtonLink>
          }
        >
          It may have been removed while you were editing. Nothing was saved.
        </Alert>
      )}
      {banner && banner.kind !== 'forbidden' && banner.kind !== 'not-found' && (
        <ApiProblemAlerts
          problem={banner}
          alertRef={alertRef}
          remaining={waiting}
        />
      )}
      {issues.length > 0 && failedSubmits > 0 && (
        <ErrorSummary key={failedSubmits} issues={issues} />
      )}

      <form
        noValidate
        onSubmit={submit}
        className="flex max-w-xl flex-col gap-5"
      >
        <TextField
          id="edit-user-fullName"
          label="Full name"
          autoComplete="off"
          isRequired
          helper="2 to 150 characters."
          value={form.values.fullName}
          readOnly={isLoading}
          error={form.errors.fullName}
          onChange={(event) => form.setValue('fullName', event.target.value)}
          onBlur={() => form.blur('fullName')}
        />
        <TextField
          id="edit-user-email"
          label="Email"
          type="email"
          readOnly
          value={user.email}
          helper="This is how they log in, so it cannot be changed here."
        />
        <TextField
          id="edit-user-phone"
          label="Phone"
          type="tel"
          autoComplete="off"
          inputMode="tel"
          isRequired={hadPhone}
          isMono
          prefix={isLocal ? '+94' : undefined}
          helper={
            isLocal
              ? 'A Sri Lankan mobile or landline number.'
              : 'Include the country code, for example +44 20 7123 4567.'
          }
          value={form.values.phone}
          readOnly={isLoading}
          error={form.errors.phone}
          onChange={(event) => form.setValue('phone', event.target.value)}
          onBlur={() => form.blur('phone')}
        />

        {isMerchant ? (
          <TextField
            id="edit-user-role-merchant"
            label="Role"
            readOnly
            value="MERCHANT"
            isMono
            helper="A merchant account keeps its role. It belongs to a merchant record."
          />
        ) : (
          <RadioGroup
            name="edit-user-role"
            legend="Role"
            isDisabled={isLoading || isSelf}
            options={roleOptions}
            value={form.values.role}
            error={form.errors.role}
            helper={
              isSelf
                ? 'You cannot change your own role.'
                : user.role === 'COURIER' && form.values.role !== 'COURIER'
                  ? 'Their courier details are kept in case they become a courier again.'
                  : undefined
            }
            onChange={(value) => form.setValue('role', value)}
          />
        )}

        {showCourierFields && (
          <fieldset className="flex flex-col gap-5">
            <legend className="mb-3 text-label font-semibold">
              Courier details
            </legend>
            <TextField
              id="edit-user-vehicleType"
              label="Vehicle type"
              autoComplete="off"
              maxLength={VEHICLE_TYPE_MAX * 4}
              helper="For example Motorbike."
              value={form.values.vehicleType}
              readOnly={isLoading}
              error={form.errors.vehicleType}
              onChange={(event) =>
                form.setValue('vehicleType', event.target.value)
              }
              onBlur={() => form.blur('vehicleType')}
            />
            <TextField
              id="edit-user-vehicleNumber"
              label="Vehicle number"
              autoComplete="off"
              isMono
              maxLength={VEHICLE_NUMBER_MAX * 4}
              value={form.values.vehicleNumber}
              readOnly={isLoading}
              error={form.errors.vehicleNumber}
              onChange={(event) =>
                form.setValue('vehicleNumber', event.target.value)
              }
              onBlur={() => form.blur('vehicleNumber')}
            />
            <Select
              id="edit-user-homeZoneId"
              label="Home zone"
              placeholder={
                currentZoneId
                  ? undefined
                  : zones.status === 'loading'
                    ? 'Loading zones'
                    : 'No home zone'
              }
              options={zoneOptions}
              disabled={isLoading || zones.status !== 'ready'}
              helper={zoneHelper}
              value={form.values.homeZoneId}
              error={form.errors.homeZoneId}
              onChange={(event) =>
                form.setValue('homeZoneId', event.target.value)
              }
            />
            <Checkbox
              id="edit-user-isAvailable"
              label="Available for assignments"
              description="A courier who is not available is left out of the dispatcher's list."
              checked={isAvailable}
              disabled={isLoading}
              onChange={(event) => setIsAvailable(event.target.checked)}
            />
          </fieldset>
        )}

        <div className="flex flex-wrap items-center gap-3">
          <Button
            type="submit"
            isLoading={isLoading}
            isDisabled={!isDirty || isWaiting}
          >
            {isLoading ? 'Saving' : 'Save changes'}
          </Button>
          <ButtonLink href="/admin/users" variant="secondary">
            Cancel
          </ButtonLink>
          {!isDirty && (
            <span className="text-caption text-text-faint">
              Change a field to save.
            </span>
          )}
        </div>
      </form>
    </>
  );
}
