---
name: Agent-Team
description: A precision-control system for bounded work and evidence-led orchestration.
colors:
  action-blue: "#4f8cff"
  verified-teal: "#3dd6ba"
  authority-amber: "#f4bd64"
  night: "#050b13"
  navy: "#081522"
  panel: "#0d1c2c"
  panel-raised: "#11263a"
  line: "#24394d"
  ink: "#f4f7fb"
  muted: "#a9b8c8"
  subtle: "#7890a7"
typography:
  display:
    fontFamily: '"Space Grotesk", "Avenir Next", "Segoe UI", sans-serif'
    fontSize: "clamp(3.8rem, 7.6vw, 7rem)"
    fontWeight: 700
    lineHeight: 0.88
    letterSpacing: "-0.04em"
  headline:
    fontFamily: '"Space Grotesk", "Avenir Next", "Segoe UI", sans-serif'
    fontSize: "clamp(2.35rem, 5vw, 4.6rem)"
    fontWeight: 700
    lineHeight: 0.96
    letterSpacing: "-0.035em"
  body:
    fontFamily: '"Space Grotesk", "Avenir Next", "Segoe UI", sans-serif'
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.65
  mono:
    fontFamily: '"SFMono-Regular", Consolas, monospace'
    fontSize: "0.82rem"
    fontWeight: 500
    lineHeight: 1.55
rounded:
  control: "10px"
  surface: "14px"
spacing:
  control-x: "1.15rem"
  surface: "1.5rem"
components:
  button-primary:
    backgroundColor: "#245fc9"
    textColor: "#ffffff"
    rounded: "{rounded.control}"
    padding: "0.75rem 1.15rem"
    height: "48px"
  panel:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.surface}"
    padding: "1.5rem"
---

# Design System: Agent-Team

## Overview

**Creative North Star: “The Precision Control Room”**

Midnight engineering surfaces frame a calm, evidence-first operating system. Mineral teal marks continuity and verified flow, cobalt carries action, and warm amber identifies gates, release authority, and keyboard focus. The generated orchestration plate is the primary image; its alt text and adjacent copy preserve the same meaning without the image.

## Colors

Blue is reserved for primary actions and links, teal for verified state and connectors, and amber for authority or caution. Near-black and blue-black layers provide structure through contrast rather than decoration; muted and subtle blue-grays carry secondary copy and captions.

**The Three-Signal Rule.** Do not interchange blue action, teal verification, and amber authority roles.

## Typography

Self-hosted Space Grotesk (`font-display: swap`, weights 400–700) carries display, navigation, and body text. Compact headings use tight tracking and line height, with the hero constrained to about nine characters per line. The system monospace is limited to commands, versions, measurements, labels, and state; the embedded workflow SVG retains its explicit Avenir/Segoe UI fallback stack.

On narrow screens, the hero display becomes `clamp(3.4rem, 16vw, 5.2rem)` and section headings become `2.7rem`. Release metadata beside the hero is deliberately reset to sentence case sans text rather than a coded uppercase label.

## Layout

The page uses a centered `1180px` maximum container with `3rem` total side clearance, editorial two-column hero and section headers, and section padding that scales from `5rem` to `8rem`. The hero pairs a slightly narrower copy column with a minimum `420px` media column; evidence grids, panels, and command cards repeat the same bounded alignment.

At `900px`, hero and section headers become single-column, three-up principles stack, and split/command grids collapse. At `680px`, side clearance becomes `1.25rem`, navigation becomes a horizontal scroll region, metrics and all flow lanes linearize, and footer/captions stack. Technical diagrams and command lines own their overflow: the workflow SVG keeps a `920px` minimum width inside a focusable horizontal scroller, while code blocks scroll without widening the page. Root overflow is clipped as a final containment guard.

## Elevation & Depth

Depth is restrained: most surfaces use a one-pixel line against tonal layers. The hero image alone receives the large ambient shadow (`0 24px 70px #0007`) and a blurred teal backglow; control, board, and diagram overrides remain flat. The sticky header uses a translucent night surface and `18px` backdrop blur.

## Shapes

Primary content surfaces and imagery use softly engineered corners (`14px`); buttons and code fields use tighter corners (`10px` or `9px`). Status dots and host labels are the only fully rounded forms. Flow geometry stays rectilinear, using thin rules, arrows, and discrete nodes.

## Components

### Buttons and links

Buttons are at least `48px` high. The primary uses the darker shipped cobalt (`#245fc9`) with white text and no shadow; hover deepens to `#1d52b2` and lifts `2px`. The secondary uses the panel surface and line border. Links underline by default and all links receive a visible `3px` amber focus outline with `4px` offset.

### Cards, boards, and callouts

Cards combine the panel fill, one-pixel line, and `14px` radius. Connected card groups use one-pixel gutters rather than independent shadows. The amber callout uses a low-opacity amber fill and border. The hero board is a compact three-stage flow that changes from horizontal arrows to a vertical sequence on mobile.

### Navigation, imagery, and technical content

The header stays sticky and the brand exposes the teal continuity signal without glow. The hero image is cropped to `8 / 5`, fills its column, and has a descriptive alt. Captions are uppercase monospace. The main workflow diagram has title/description semantics, an explicit focus label, and an on-mobile scroll prompt plus inset teal edge cue.

## Do's and Don'ts

- **Do** preserve the distinct semantic roles of blue, teal, and amber.
- **Do** keep diagrams, commands, and navigation inside their own scroll regions on small screens.
- **Do** retain the skip link, semantic image descriptions, visible focus indicators, and `prefers-reduced-motion` treatment; reduced motion disables smooth scrolling, transitions, and hover lift.
- **Don't** make page meaning depend on the generated image or workflow SVG.
- **Don't** add ambient shadows to routine cards or restore glow to status marks.
- **Don't** let technical labels or long commands force document-level horizontal overflow.
