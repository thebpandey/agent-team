# UI/UX direction and implementation

Load only for UI/UX tasks. Use Impeccable for the orchestrator and involved teammates when available and enabled. If declined/unavailable, continue with the structured workflow below using existing design tokens, semantic components, responsive layouts, and rendered inspection. Do not claim Impeccable ran. Preserve approved design systems. For new systems, establish audience, primary user job, brand constraints, visual direction, and representative content before decoration. Ask only about material unresolved decisions; do not restart discovery for known facts.

## Structured, bounded workflow

1. Inspect relevant product/design docs and interface. Establish one canonical specification for typography, colors, spacing, layout, component variants, interaction/motion, and accessibility. Update existing docs rather than creating competing ledgers. If tools emit separate files, designate one authoritative source and link rather than copy tokens.
2. Use applicable Impeccable planning/build guidance when available, otherwise the steps in this reference. For broad new designs, optionally consult UI UX Pro Max for a targeted palette, typography, pattern, or stack recommendation. Do not load its entire library or create a competing system. Avoid stacking aesthetic skills without a specific unresolved need.
3. Implement a representative page/component using shared tokens, then reuse its patterns. Favor hierarchy, typography, deliberate spacing, and product-specific content. Avoid generic decoration and unnecessary animation. Retain approved fonts/colors even if generic guidance dislikes them.
4. Inspect rendered output at representative narrow/wide viewports. Check the changed flow, keyboard focus, contrast, readable text, overflow, relevant loading/empty/error states, and reduced motion for animations. Capture screenshots and revision-linked evidence. Automated accessibility checks supplement practical inspection.
5. Combine technical/visual findings into the existing focused review. Fix concrete issues, verify affected output, and stop when criteria pass. Do not run every Impeccable command or start open-ended polish cycles. Use expensive effects only when they serve the brief and fit actual performance constraints.

## Curated references

Researched 2026-09-04. Approximate GitHub stars observed during research indicate adoption, not quality ratings or guarantees. Recommendations reflect workflow fit; do not refresh this catalog during ordinary implementation. Verify current license/compatibility when introducing selected components. Reference upstream skills rather than copying their instructions into this package.

| Resource | Role | License / observed stars |
| --- | --- | --- |
| [Impeccable](https://github.com/pbakaus/impeccable) | Preferred when available: product/design context, direction, responsive adaptation, critique, refinement; use relevant commands only | Apache-2.0; 65.6k |
| [UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) | Optional broader design exploration and persistent specifications; searchable palettes, typography, patterns, UX and stack guidance; adds setup/context cost | MIT; 125k |
| [UI Skills](https://github.com/ibelick/ui-skills) | Optional targeted design-engineering guidance; retrieve the relevant skill, not the registry | MIT repository; 8.1k; verify individual external skill terms |
| [shadcn/ui](https://github.com/shadcn-ui/ui) | Accessible component foundation for compatible projects; customize with product tokens | MIT; 123k |
| [Magic UI](https://github.com/magicuidesign/magicui) | Selected ready-made animated components/accents from the free repository; do not assume access to commercial offerings | MIT repository; 22.2k |
| [Motion](https://github.com/motiondivision/motion) | Purposeful interaction/layout animation when CSS is insufficient; core library only, with Motion+ AI skills/premium examples separate | MIT core; 33.5k |
| [React Bits](https://github.com/DavidHDev/react-bits) | Optional expressive backgrounds, text, interactions; check suitability/performance | MIT + Commons Clause; 46.8k; additional restrictions, not unrestricted MIT |

Default combination: available Impeccable plus existing project components; without it, use the built-in workflow above. For a compatible new React app, consider shadcn/ui and selected Magic UI components or Motion only where useful. Consult UI UX Pro Max when a larger design decision needs its structured data. For Taste Skill, Bklit charts, DESIGN.md examples, and procedural 3D, use the optional routing reference linked from SKILL.md. Do not install every option or impose React on other stacks.

The harness contains dependency-use instructions and references, not third-party skill copies, component code, model access, or paid services. Report availability honestly.
