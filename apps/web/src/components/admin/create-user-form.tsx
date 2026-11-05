'use client';

import { Lock } from 'lucide-react';
import { useRouter } from 'next/navigation';
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react';

import {
  createAdminUser,
  type CreateableRole,
  type NewStaffUser,
} from '../../lib/admin-users-api';
import { ApiError } from '../../lib/api';
import { classifyApiError, type ApiProblem } from '../../lib/api-problem';
import { getZones, type Zone } from '../../lib/pricing-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  serverFieldMessage,
  toSriLankanE164,
  validateEmail,
  validateFullName,
  validateNewPassword,
  validatePhone,
} from '../../lib/validation';
import { ApiProblemAlerts } from '../auth/api-problem-alerts';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { ErrorSummary, type ErrorSummaryIssue } from '../ui/error-summary';
import { TextField } from '../ui/field';
import { PasswordField } from '../ui/password-field';
import { RadioGroup } from '../ui/radio-group';
import { Select } from '../ui/select';
import { useToast } from '../ui/toast';

type Values = {
  fullName: string;
  email: string;
  phone: string;
  role: string;
  password: string;
  vehicleType: string;
  vehicleNumber: string;
  homeZoneId: string;
};
type Field = keyof Values;

/** The longest the API accepts (identity service, courier profile). */
const VEHICLE_TYPE_MAX = 30;
const VEHICLE_NUMBER_MAX = 20;

const roleOptions: {
  value: CreateableRole;
  label: string;
  description: string;
}[] = [
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

const isCourier = (values: Values) => values.role === 'COURIER';

const rules: FormRules<Values> = {
  fullName: (v) => validateFullName(v.fullName),
  email: (v) => validateEmail(v.email),
  phone: (v) => validatePhone(v.phone),
  role: (v) => (v.role === '' ? 'Choose a role.' : undefined),
  password: (v) => validateNewPassword(v.password),
  vehicleType: (v) =>
    isCourier(v) && [...v.vehicleType.trim()].length > VEHICLE_TYPE_MAX
      ? `Use ${VEHICLE_TYPE_MAX} characters or fewer.`
      : undefined,
  vehicleNumber: (v) =>
    isCourier(v) && [...v.vehicleNumber.trim()].length > VEHICLE_NUMBER_MAX
      ? `Use ${VEHICLE_NUMBER_MAX} characters or fewer.`
      : undefined,
  homeZoneId: () => undefined,
};

const labels: Record<Field, string> = {
  fullName: 'Full name',
  email: 'Email',
  phone: 'Phone',
  role: 'Role',
  password: 'Initial password',
  vehicleType: 'Vehicle type',
  vehicleNumber: 'Vehicle number',
  homeZoneId: 'Home zone',
};

/** The id of each field's first focusable control. */
const fieldId = (field: Field) =>
  field === 'role' ? 'create-user-role-COURIER' : `create-user-${field}`;

/** The API's field names for the courier profile, and our own for the rest. */
const serverFields: Record<string, Field> = {
  fullName: 'fullName',
  email: 'email',
  phone: 'phone',
  role: 'role',
  password: 'password',
  'courierProfile.vehicleType': 'vehicleType',
  'courierProfile.vehicleNumber': 'vehicleNumber',
  'courierProfile.homeZoneId': 'homeZoneId',
};

type Zones =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ready'; zones: Zone[] }
  | { status: 'error' };

type Banner = ApiProblem | { kind: 'forbidden' };

function requestOf(values: Values): NewStaffUser {
  const role = values.role as CreateableRole;
  const request: NewStaffUser = {
    fullName: values.fullName.trim(),
    email: values.email.trim(),
    phone: toSriLankanE164(values.phone),
    role,
    password: values.password,
  };
  if (role === 'COURIER') {
    // Anything left blank stays out of the request: the API treats it as not given.
    request.courierProfile = {};
    if (values.vehicleType.trim())
      request.courierProfile.vehicleType = values.vehicleType.trim();
    if (values.vehicleNumber.trim())
      request.courierProfile.vehicleNumber = values.vehicleNumber.trim();
    if (values.homeZoneId)
      request.courierProfile.homeZoneId = values.homeZoneId;
  }
  return request;
}

/**
 * The create-user form of /admin/users/new. Administrators create couriers,
 * dispatchers and other administrators here; customers and merchants register
 * themselves.
 */
export function CreateUserForm(): ReactNode {
  const router = useRouter();
  const { recheck } = useSession();
  const { show } = useToast();

  const form = useForm<Values>(
    {
      fullName: '',
      email: '',
      phone: '',
      role: '',
      password: '',
      vehicleType: '',
      vehicleNumber: '',
      homeZoneId: '',
    },
    rules,
  );
  const [banner, setBanner] = useState<Banner | null>(null);
  const [failedSubmits, setFailedSubmits] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [zones, setZones] = useState<Zones>({ status: 'idle' });
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    banner && 'seconds' in banner ? banner.seconds : null,
  );
  const isWaiting = banner?.kind === 'rate-limited' && waiting > 0;
  const showCourierFields = isCourier(form.values);

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
    (field) =>
      showCourierFields ||
      !['vehicleType', 'vehicleNumber', 'homeZoneId'].includes(field),
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

    setIsLoading(true);
    try {
      const created = await createAdminUser(requestOf(form.values));
      show({ title: `${created.fullName}'s account was created.` });
      router.push('/admin/users');
    } catch (error) {
      setIsLoading(false);
      handleFailure(error);
    }
  }

  function handleFailure(error: unknown) {
    if (error instanceof ApiError) {
      if (error.code === 'EMAIL_IN_USE') {
        form.setServerErrors({
          email: 'An account with this email already exists.',
        });
        setFailedSubmits((count) => count + 1);
        return;
      }
      if (error.code === 'FORBIDDEN') {
        setBanner({ kind: 'forbidden' });
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

  const zoneOptions =
    zones.status === 'ready'
      ? zones.zones.map((zone) => ({ value: zone.id, label: zone.name }))
      : [];
  const zoneHelper =
    zones.status === 'error'
      ? 'We could not load the zones. You can create the courier without one and set it later.'
      : zones.status === 'ready' && zones.zones.length === 0
        ? 'There are no zones yet. You can set one later.'
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
          Only administrators can create users. If you need access, ask an
          administrator.
        </Alert>
      )}
      {banner && banner.kind !== 'forbidden' && (
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
          id={fieldId('fullName')}
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
          id={fieldId('email')}
          label="Email"
          type="email"
          autoComplete="off"
          isRequired
          helper="They use this to log in."
          value={form.values.email}
          readOnly={isLoading}
          error={form.errors.email}
          onChange={(event) => form.setValue('email', event.target.value)}
          onBlur={() => form.blur('email')}
        />
        <TextField
          id={fieldId('phone')}
          label="Phone"
          type="tel"
          autoComplete="off"
          inputMode="tel"
          isRequired
          isMono
          prefix="+94"
          helper="A Sri Lankan mobile or landline number."
          value={form.values.phone}
          readOnly={isLoading}
          error={form.errors.phone}
          onChange={(event) => form.setValue('phone', event.target.value)}
          onBlur={() => form.blur('phone')}
        />
        <RadioGroup
          name="create-user-role"
          legend="Role"
          isRequired
          isDisabled={isLoading}
          options={roleOptions}
          value={form.values.role}
          error={form.errors.role}
          onChange={(value) => {
            form.setValue('role', value);
            form.blur('role');
          }}
        />
        <PasswordField
          id={fieldId('password')}
          label="Initial password"
          autoComplete="new-password"
          isRequired
          showChecklist
          helper="Share this password with them securely. The email we send does not include it."
          value={form.values.password}
          readOnly={isLoading}
          error={form.errors.password}
          onChange={(event) => form.setValue('password', event.target.value)}
          onBlur={() => form.blur('password')}
        />

        {showCourierFields && (
          <fieldset className="flex flex-col gap-5">
            <legend className="mb-3 text-label font-semibold">
              Courier details
            </legend>
            <TextField
              id={fieldId('vehicleType')}
              label="Vehicle type"
              autoComplete="off"
              maxLength={VEHICLE_TYPE_MAX * 4}
              helper="Optional, for example Motorbike."
              value={form.values.vehicleType}
              readOnly={isLoading}
              error={form.errors.vehicleType}
              onChange={(event) =>
                form.setValue('vehicleType', event.target.value)
              }
              onBlur={() => form.blur('vehicleType')}
            />
            <TextField
              id={fieldId('vehicleNumber')}
              label="Vehicle number"
              autoComplete="off"
              isMono
              maxLength={VEHICLE_NUMBER_MAX * 4}
              helper="Optional."
              value={form.values.vehicleNumber}
              readOnly={isLoading}
              error={form.errors.vehicleNumber}
              onChange={(event) =>
                form.setValue('vehicleNumber', event.target.value)
              }
              onBlur={() => form.blur('vehicleNumber')}
            />
            <Select
              id={fieldId('homeZoneId')}
              label="Home zone"
              placeholder={
                zones.status === 'loading' ? 'Loading zones' : 'No home zone'
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
          </fieldset>
        )}

        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" isLoading={isLoading} isDisabled={isWaiting}>
            {isLoading ? 'Creating' : 'Create account'}
          </Button>
          <ButtonLink href="/admin/users" variant="secondary">
            Cancel
          </ButtonLink>
        </div>
      </form>
    </>
  );
}
