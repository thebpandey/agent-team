# Select optional design tools

Use this guide only when the task needs an extra design tool. Prefer tools already used by the project. A missing optional tool must not stop ordinary work. Do not install a complete collection or start a paid service automatically.

## Choose by task

| Tool | Use it for | Do not add it when | Conditions |
| --- | --- | --- | --- |
| [Motion](https://motion.dev/) | Movement that helps users understand changes on a screen. | Simple page styles or an installed tool already meet the need. | The core package uses the MIT license. Motion+ features require separate access. |
| [Taste Skill](https://www.tasteskill.dev/) | A distinct style for landing pages, portfolios, or a major visual redesign. | The task is a small repair, data table, dashboard, or multi-step app process. | Use the general `design-taste-frontend` skill. Inspect the selected revision before use. |
| [img2threejs](https://github.com/img2threejs/img2threejs) | A 3D object or scene based on reference images. | A normal image or existing 3D model meets the need. | It uses Apache-2.0. It needs reference images and browser tools. An exact shape is not guaranteed. |
| [Awesome DESIGN.md](https://github.com/VoltAgent/awesome-design-md) | Examples of colors, fonts, spacing, and page layouts. | The project already has an approved design guide. | The collection uses MIT. It does not grant permission to copy other companies' brand assets. |
| [Bklit UI](https://github.com/bklit/bklit-ui) | Charts that show app data, totals, and changes over time. | The app needs no charts or already has suitable charts. | The chart components use MIT. The private Studio source code is excluded. |

Other optional tools are explained in [the tool guide](dependencies.md). Use [the UI procedure](ui.md) for the main design steps.

## Keep the design process short

Use Impeccable when available and enabled. Otherwise, use the built-in UI procedure. Assign each extra tool one purpose. Record its version and purpose once. Share the approved design decisions with teammates. Do not repeat the same search for each agent.

Normally, use no more than one extra tool for broad design advice. Add another only to resolve a specific unmet requirement. Do not start separate review rounds for Impeccable, Taste, and Pro Max. Follow user requirements, approved branding, and accessibility requirements. Accessibility means that people with different abilities can use the product.

For Taste, load only the general skill for a suitable task. Do not load its full instructions for every teammate by default. Do not automatically select `gpt-taste`. That variant adds specific animation and marketing rules that can exceed the task. Never claim that an unrun script ran. Do not add themes or animation only because a generic instruction suggests them. See the [general skill](https://github.com/Leonxlnx/taste-skill/blob/main/skills/taste-skill/SKILL.md) and [separate GPT variant](https://github.com/Leonxlnx/taste-skill/blob/main/skills/gpt-tasteskill/SKILL.md).

For Awesome DESIGN.md, inspect one or two relevant examples. Add useful rules to the project's approved design guide. Keep source credits. Preserve the project's own brand. Do not copy the full collection into that guide. Do not assume that an example is an official brand specification.

For Bklit, use its [skill](https://bklit.com/docs/skills) only for chart work. Check the existing shadcn setup. Reuse installed charts where possible. Keep correct scales, units, and clear labels. Provide keyboard access and touch controls, or a text version of the data. Reduce movement when the user requests it. Exclude the contributor's Studio and release procedures. See the [license details](https://github.com/bklit/bklit-ui#license).

For img2threejs, define the required object and acceptable accuracy before work starts. Follow its local progress checks and correction limits. Do not erase its state to bypass a stop. Link its files and results from Beads or TASKS.md. Keep one project task record. Preserve the tool's own files so work can resume. Check the actual browser result and expected device performance. Use a static image if appropriate. Do not add judges or review panels. See the [tool procedure](https://github.com/img2threejs/img2threejs/blob/main/SKILL.md).

For Motion, use simple page styles when sufficient. Otherwise, reuse an installed animation tool before adding another. Use commands supported by the installed version. Keep movement useful. Respect the user's reduced-motion setting. Do not take control of page scrolling without a specific approved need. See the [product page](https://motion.dev/) for the free package and paid products.

Include these checks in the existing development review. Stop when the agreed requirements and concrete findings are resolved.
