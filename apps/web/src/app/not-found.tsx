import Link from "next/link";
import { BrandMark, Icon } from "@paritylab/ui";

export default function NotFound() {
  return <main className="not-found"><BrandMark/><div className="not-found__signal"><span>4</span><i/><span>4</span></div><h1>This event left the trace.</h1><p>The route does not exist, but the system is still converged.</p><Link href="/" className="cta cta--primary">Return home <Icon name="arrow" /></Link></main>;
}
