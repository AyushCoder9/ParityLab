# Optical Ledger design system

## Theme

A payment integration examined on a calibrated optical bench: bright forensic clarity, brief obsidian fault space, and translucent event paths that reconverge into verified state.

## Color

All authored colors use OKLCH.

```css
--canvas: oklch(1 0 0);
--canvas-dark: oklch(0.09 0 0);
--surface: oklch(0.975 0.006 160);
--surface-dark: oklch(0.14 0.008 160);
--ink: oklch(0.13 0.01 160);
--ink-inverse: oklch(0.97 0.004 160);
--muted: oklch(0.47 0.018 160);
--line: oklch(0.89 0.01 160);
--primary: oklch(0.39 0.105 160);
--verified: oklch(0.68 0.13 160);
--fault: oklch(0.64 0.19 30);
--warning: oklch(0.76 0.15 78);
```

## Typography

- Spline Sans: display, body, labels, controls.
- Martian Mono: event IDs, timestamps, payloads, code, tabular data.
- Marketing display maximum: 6rem. Product UI uses a fixed compact scale.

## Layout

- Marketing: asymmetric full-bleed chapters, one idea per fold.
- Product: 16-column desktop grid, compact navigation, resizable evidence panels.
- Cards are used only for independently actionable groups; avoid nested cards.

## Signature

The event braid maps browser, API, Stripe object, webhook, worker, and database paths through invariant checkpoints. It must be meaningful, inspectable, and generated from run data.

## Motion

- Feedback: 100–150ms.
- Product state: 150–250ms.
- Drawers/layout: 250–400ms.
- Marketing chapters: 500–900ms, scroll-controlled where appropriate.
- Exits are approximately 25% faster than entrances.
- No decorative animation for keyboard actions.
