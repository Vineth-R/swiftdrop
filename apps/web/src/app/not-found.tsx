import { SystemPage } from '../components/layout/system-page';
import { ButtonLink } from '../components/ui/button';

export default function RootNotFound() {
  return (
    <main className="flex min-h-screen flex-col bg-paper px-4">
      <SystemPage
        label="404 · Not found"
        title="This page does not exist"
        actions={
          <>
            <ButtonLink href="/">Go to the home page</ButtonLink>
            <ButtonLink href="/login" variant="secondary">
              Log in
            </ButtonLink>
          </>
        }
      >
        The link may be out of date, or a tracking number may have been
        mistyped.
      </SystemPage>
    </main>
  );
}
