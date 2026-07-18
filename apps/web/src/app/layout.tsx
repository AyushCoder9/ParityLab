import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "ParityLab — Prove your Stripe integration", template: "%s · ParityLab" },
  description: "Continuous verification for Stripe integrations under duplicate delivery, event disorder, retries and endpoint faults.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
