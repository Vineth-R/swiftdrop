import './global.css';

export const metadata = {
  title: 'SwiftDrop — Delivery operations',
  description: 'A clear view of every SwiftDrop delivery.',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
