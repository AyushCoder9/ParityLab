"use client";

import { useEffect, useId, useState } from "react";
import type { EventStatus } from "@/lib/simulation";

type BraidMode = "healthy" | "fault" | "verified";

export function EventBraid({ mode = "healthy", progress = 1, compact = false, labels = true }: { mode?: BraidMode; progress?: number; compact?: boolean; labels?: boolean }) {
  const uid = useId().replaceAll(":", "");
  const [reducedMotion, setReducedMotion] = useState(true);

  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  const clippedProgress = Math.max(0.02, Math.min(1, progress));
  const strands = [
    { key: "browser", name: "Browser", color: "var(--braid-a)", d: "M18 70 C130 70 132 30 236 30 S340 108 455 80 S610 20 742 54 S850 100 982 52" },
    { key: "api", name: "API", color: "var(--braid-b)", d: "M18 104 C120 104 142 145 242 134 S360 53 468 87 S590 148 724 112 S856 48 982 88" },
    { key: "stripe", name: "Stripe", color: "var(--braid-c)", d: "M18 138 C116 138 150 84 253 96 S355 174 486 138 S601 66 724 88 S860 150 982 126" },
    { key: "webhook", name: "Webhook", color: "var(--braid-d)", d: "M18 172 C126 172 144 118 256 122 S362 208 487 174 S606 108 726 138 S864 198 982 160" },
  ];
  const faultPath = "M455 80 C520 80 532 16 605 20 S700 112 770 105";

  return (
    <figure className={`event-braid ${compact ? "event-braid--compact" : ""}`} aria-label={`Payment event topology: ${mode}`}>
      <svg viewBox="0 0 1000 224" role="img" aria-labelledby={`${uid}-title ${uid}-desc`}>
        <title id={`${uid}-title`}>Payment event braid</title>
        <desc id={`${uid}-desc`}>Browser, API, Stripe and webhook event paths crossing verification checkpoints.</desc>
        <defs>
          <linearGradient id={`${uid}-fade`} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" stopColor="white" stopOpacity="0" />
            <stop offset=".08" stopColor="white" />
            <stop offset=".92" stopColor="white" />
            <stop offset="1" stopColor="white" stopOpacity="0" />
          </linearGradient>
          <filter id={`${uid}-soft`} x="-10%" y="-40%" width="120%" height="180%">
            <feGaussianBlur stdDeviation="6" />
          </filter>
          <clipPath id={`${uid}-progress`}>
            <rect width={1000 * clippedProgress} height="224" />
          </clipPath>
        </defs>

        <g className="braid-grid" aria-hidden="true">
          {[120, 250, 380, 510, 640, 770, 900].map((x) => <path key={x} d={`M${x} 10v204`} />)}
          {[42, 82, 122, 162, 202].map((y) => <path key={y} d={`M8 ${y}h984`} />)}
        </g>

        <g clipPath={`url(#${uid}-progress)`}>
          {strands.map((strand) => (
            <g key={strand.key} className="braid-strand">
              <path d={strand.d} stroke={strand.color} className="braid-glow" filter={`url(#${uid}-soft)`} />
              <path d={strand.d} stroke={strand.color} className="braid-line" />
              {!reducedMotion && (
                <circle r="3.5" fill={strand.color} className="braid-particle">
                  <animateMotion dur={`${5.5 + strands.indexOf(strand) * .7}s`} repeatCount="indefinite" path={strand.d} />
                </circle>
              )}
            </g>
          ))}
          {mode !== "healthy" && (
            <g className="fault-branch">
              <path d={faultPath} className="fault-glow" filter={`url(#${uid}-soft)`} />
              <path d={faultPath} className="fault-line" />
              <circle cx="604" cy="20" r="5" className="fault-node" />
            </g>
          )}
        </g>

        {[250, 510, 770].map((x, index) => (
          <g key={x} transform={`translate(${x} 0)`} className="checkpoint">
            <circle cy={index === 1 && mode === "fault" ? 88 : index === 2 ? 106 : 106} r="10" />
            <circle cy={index === 1 && mode === "fault" ? 88 : index === 2 ? 106 : 106} r="3" />
          </g>
        ))}

        {labels && (
          <g className="braid-labels">
            <text x="20" y="25">PAYMENT SURFACE</text>
            <text x="842" y="208">MERCHANT STATE</text>
            <text x="228" y="213">request</text>
            <text x="485" y="213">delivery</text>
            <text x="738" y="213">invariant</text>
          </g>
        )}
      </svg>
    </figure>
  );
}

export function EventStatusGlyph({ status }: { status: EventStatus }) {
  return <span className={`event-status event-status--${status}`} aria-hidden="true"><span /></span>;
}
