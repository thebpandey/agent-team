# Design and check app screens

UI means the screens and controls that a person uses. UX means how easy the product is to understand and use. Read this guide for UI/UX tasks only.

Use Impeccable for the lead agent and relevant teammates when available and enabled. Otherwise, use the procedure below. Do not claim that a missing skill ran. Preserve approved colors, fonts, and design rules.

For a new design, identify the users and their main task. Establish the brand, visual direction, and representative content. Ask about unresolved decisions only when they affect the result.

## Design procedure

1. Read the existing design guide and inspect the app. Keep one approved design guide for colors, fonts, spacing, layout, controls, movement, and accessibility.
2. Use the relevant Impeccable instructions when available. Use UI UX Pro Max only when a specific design question needs more examples.
3. Build one representative page or component. Reuse its approved patterns. Use clear text and spacing. Avoid decoration that does not help the user.
4. Follow the Pro [visual browser review](visual-review.md) procedure for changed UI. Assign the existing reviewer unless substantial UI work needs a separate visual tester. Inspect actual screenshots at representative small and large sizes. Check the changed user flow. Record evidence and the tested revision.
5. Include design findings in the existing review. Repair concrete problems. Check the affected result. Stop when the agreed criteria pass.

A design token is a named value for a color, font, or size. Keep these values in one approved source. Link tool-generated files to that source instead of creating competing design guides.

Accessibility means that people with different abilities can use the product. Automatic checks support direct inspection; they do not replace it. Use complex visual effects only when they meet the brief and work on expected devices.

## Select tools

| Tool | Purpose | Selection rule |
| --- | --- | --- |
| [Impeccable](https://github.com/pbakaus/impeccable) | Helps plan, improve, and check app screens. | Preferred when available. Run only the instructions relevant to the task. |
| [UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) | Supplies searchable design examples. | Use it for a specific unanswered design question. |
| [UI Skills](https://github.com/ibelick/ui-skills) | Supplies separate design procedures. | Load only the relevant skill. |
| [shadcn/ui](https://github.com/shadcn-ui/ui) | Supplies reusable screen controls. | Use it when the project supports it. Keep the project's colors and fonts. |
| [Magic UI](https://github.com/magicuidesign/magicui) | Supplies visual effects and animated components. | Select useful free components only. |
| [Motion](https://github.com/motiondivision/motion) | Adds movement to screen parts. | Use it when simple page styles are insufficient. |
| [React Bits](https://github.com/DavidHDev/react-bits) | Adds text, background, and interaction effects. | Check the effect's need, speed, and license conditions. |

These are task choices, not quality ratings. Check the selected version and license when adding a tool. Do not repeat this research during ordinary work. External tools retain their own licenses. React Bits uses MIT + Commons Clause. Motion+ and commercial offerings need separate access.

The default is available Impeccable plus existing project components. Without Impeccable, use the procedure above. Do not change the project's software only to use an optional component. For Taste Skill, Bklit, DESIGN.md examples, or 3D work, read [the optional tool guide](ui-optional.md).

This package contains instructions and source links. It does not include the external tools, paid services, or AI model access.
