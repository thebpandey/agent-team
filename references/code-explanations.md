# Pro explanations beside feature code

When an agent builds or changes a feature, add or update concise documentation beside the relevant code. This Pro procedure is not part of Agent Team Lite. The purpose is to help a non-professional developer understand what a section does and why it exists.

## Explain the objective

Use the language's normal comments or documentation strings. Place an explanation above the feature's entry point or main function. Add another at a distinct section when its purpose, business rule, or side effect is not clear. One accurate comment can cover a small feature. Do not comment on every line.

Write one to three short sentences. Use plain English and ASD-STE100 principles. Keep official identifiers unchanged. Explain a necessary technical term when the reader first needs it. State the objective, important result, and any non-obvious reason for the behavior.

Prefer clear names and simple structure first. Comments must describe the actual implementation, not an intended feature that is still missing. Do not add claims such as “secure,” “fast,” or “guaranteed” without a precise supported meaning.

Example:

```ts
// Record this learner's completed lesson so progress remains after a reload.
// Reuse an existing completion record to prevent duplicate progress entries.
await saveLessonCompletion(learnerId, lessonId);
```

Avoid comments that merely repeat syntax, such as “Set count to zero.” Explain important assumptions, data changes, external calls, and error handling when they affect the feature. Keep longer architecture explanations in the existing project documentation and link them from the code when useful.

## Keep explanations accurate

- Update the explanation whenever the relevant behavior changes. Remove a stale statement when it no longer applies.
- Follow project comment style. Preserve useful existing documentation rather than duplicating it.
- Do not put comments into formats that reject them, such as strict JSON. Use the nearest supported documentation file and identify the exact file and section.
- Do not edit generated or vendor files just to add comments. Document the owning source, generator, or integration boundary instead.
- Do not include credentials, private customer data, task transcripts, or temporary debugging notes in code comments.
- Do not copy the same explanation across multiple files. Put it where a reader needs it and use a short reference elsewhere.

## Add a feature guide only when useful

For a feature that spans several files or needs setup steps, update the existing feature guide, README, or user guide. Create a short feature guide only if no suitable guide exists. A small feature usually needs only the nearby code explanation. Do not require a separate guide for every feature.

Describe the purpose, main user steps, file responsibilities, important rules, and a practical verification step. Document configuration or user instructions where the project's readers expect them. Keep one authoritative explanation and link to it instead of copying it. Use actual paths and behavior, not the illustrative paths below.

Example feature guide:

```markdown
# Lesson completion

## Purpose
Remember which lessons a user has completed.

## How it works
1. The user selects "Mark complete."
2. The server checks that the user is signed in.
3. The server saves the completion record.
4. The page shows confirmation after the save succeeds.

## Main files
- components/CompleteLessonButton.tsx: Handles the button.
- app/api/progress/route.ts: Checks and saves the request.
- lib/progress.ts: Reads and writes completion records.

## Important behavior
Selecting the button again does not create another record.
If saving fails, the page offers another attempt.

## Verification
Complete a lesson, reload the page, and confirm that it
still appears as completed.
```

## Include this in the existing review

The developer reports the documented file or section with the feature handoff. The orchestrator records that location in the existing task record. The reviewer checks that the explanation is present, clear, concise, and consistent with the code. Repair misleading documentation in the same review loop. Do not create a documentation agent or a separate review pass for ordinary feature work.

A documentation-only repair needs a focused syntax or format check if relevant. Do not rerun the full app suite only because a comment changed. Required project checks still apply.
