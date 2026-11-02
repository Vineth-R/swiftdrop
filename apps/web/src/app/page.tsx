import { ButtonLink } from '../components/ui/button';
import { Logo } from '../components/ui/logo';
import { ThemeSwitch } from '../components/ui/theme-switch';

export default function Index() {
  return (
    <main className="relative flex min-h-screen flex-col items-center justify-center gap-8 px-4">
      <div className="absolute right-4 top-4 md:right-6 md:top-6">
        <ThemeSwitch variant="compact" />
      </div>
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
