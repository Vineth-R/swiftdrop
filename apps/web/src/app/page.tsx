import { ButtonLink } from '../components/ui/button';
import { Logo } from '../components/ui/logo';

export default function Index() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-8 px-4">
      <Logo size="lg" />
      <div className="flex gap-3">
        <ButtonLink href="/login">Log in</ButtonLink>
        <ButtonLink href="/register" variant="secondary">
          Create an account
        </ButtonLink>
      </div>
    </main>
  );
}
