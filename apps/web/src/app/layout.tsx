import localFont from 'next/font/local';

import { AiChatWidget } from '../components/ai-chat/chat-widget';
import { ToastProvider } from '@/components/ui/toast';
import { THEME_INIT } from '@/lib/theme';
import './globals.css';

export const metadata = {
  title: 'SwiftDrop — Delivery operations',
  description: 'A clear view of every SwiftDrop delivery.',
};

// Self-hosted variable fonts (see ./fonts/LICENSE.md). next/font/google asked
// Google for the files at build time, and Google sometimes answers with a URL
// format that Next 16's Turbopack cannot read, which broke builds at random.
const bricolage = localFont({
  src: './fonts/bricolage-grotesque-latin.woff2',
  display: 'swap',
  variable: '--font-bricolage',
  weight: '400 800',
});
const instrument = localFont({
  src: './fonts/instrument-sans-latin.woff2',
  display: 'swap',
  variable: '--font-instrument',
  weight: '400 700',
});
const jetbrains = localFont({
  src: './fonts/jetbrains-mono-latin.woff2',
  display: 'swap',
  variable: '--font-jetbrains',
  weight: '400 700',
});

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${bricolage.variable} ${instrument.variable} ${jetbrains.variable}`}
    >
      <head>
        {/* sets data-theme before paint, so there is no flash of the wrong theme */}
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body>
        <ToastProvider>{children}</ToastProvider>
        <AiChatWidget />
      </body>
    </html>
  );
}
