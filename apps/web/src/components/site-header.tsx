import Link from "next/link";
import { BrandMark, Icon } from "@paritylab/ui";

export function SiteHeader({ dark = false }: { dark?: boolean }) {
  return (
    <header className={`site-header ${dark ? "site-header--dark" : ""}`}>
      <Link href="/" className="brand-link" aria-label="ParityLab home"><BrandMark /></Link>
      <nav aria-label="Main navigation">
        <Link href="/#system">System</Link>
        <Link href="/#evidence">Evidence</Link>
        <Link href="/demo">Simulation</Link>
      </nav>
      <Link href="/dashboard" className="header-cta">Open console <Icon name="arrow" /></Link>
    </header>
  );
}
