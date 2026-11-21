import { SystemPage } from '../../components/layout/system-page';
import { ButtonLink } from '../../components/ui/button';

export default function NotFound() {
  return (
    <SystemPage
      label="404 · Not found"
      title="This page does not exist"
      actions={<ButtonLink href="/dashboard">Go to my dashboard</ButtonLink>}
    >
      The link may be out of date, or the screen may not be built yet.
    </SystemPage>
  );
}
