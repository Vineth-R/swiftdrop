import {
  Bricolage_Grotesque,
  Instrument_Sans,
  JetBrains_Mono,
} from 'next/font/google';

import { THEME_INIT } from '../lib/theme';
import './globals.css';

export const metadata = {
  title: 'SwiftDrop — Delivery operations',
  description: 'A clear view of every SwiftDrop delivery.',
};

const bricolage = Bricolage_Grotesque({
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-bricolage',
  weight: ['400', '600', '700'],
});
const instrument = Instrument_Sans({
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-instrument',
  weight: ['400', '500', '600', '700'],
});
const jetbrains = JetBrains_Mono({
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-jetbrains',
  weight: ['400', '500', '600'],
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
      <body>{children}</body>
    </html>
  );
}
