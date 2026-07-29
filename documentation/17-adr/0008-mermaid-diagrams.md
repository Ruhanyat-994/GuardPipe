# ADR-0008 — Mermaid-in-Markdown for all diagrams

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M1 |
| Supersedes | — |

## Context

The documentation set contains roughly 30 diagrams: C4 context and container views, component maps, sequence diagrams, state machines, an ER diagram, a Gantt chart, and several flowcharts. They will change as the design evolves over four weeks.

## Options considered

### Option A — Mermaid in fenced code blocks
Text-based, rendered natively by GitHub.

### Option B — draw.io / Excalidraw, exported as PNG or SVG

### Option C — PlantUML

### Option D — Figma

## Decision

**Option A — Mermaid, in ```` ```mermaid ```` fenced blocks inside the Markdown files.**

## Rationale

The decisive property is that **diagrams are diffable in pull requests.** A change to a component diagram shows up as changed lines that a reviewer can read and comment on. An exported PNG shows up as "binary file changed" — a reviewer either takes it on faith or opens two images side by side. Over three weeks of evolving architecture, reviewable diagrams are worth substantially more than prettier ones.

GitHub renders Mermaid natively, so there is no build step, no CI job, no generated-artifact staleness, and no risk that the diagram in the repository does not match the diagram in the file that produced it — a problem every image-export workflow eventually has.

PlantUML is more expressive and produces better-looking output, but requires either a rendering service or a local Java toolchain, and GitHub does not render it inline. That is a build step and a dependency for a benefit we do not need.

draw.io and Excalidraw produce nicer diagrams. They also produce binary artifacts, a second tool everyone must install, and a source-of-truth question when the `.drawio` file and the exported `.png` drift. For a security project where the documentation lives in git, keeping everything in text is the coherent choice.

Figma remains the tool for **UI design** ([09 — Design System](../09-ui-ux-design-system.md)) — that is a different job, with different requirements, and Figma is right for it.

## Consequences

### Positive
- Diagrams render inline on GitHub with no tooling.
- Fully diffable and reviewable in pull requests.
- Editable by anyone with a text editor — no tool to install.
- Impossible for a diagram to drift from its source, because they are the same thing.
- Small repository footprint.
- Reviewable in the same PR as the code the diagram describes.

### Negative
- **Less control over layout.** Mermaid's auto-layout occasionally produces awkward arrangements. Accepted — clarity matters more than beauty here.
- Limited styling compared to a drawing tool.
- Complex diagrams become hard to read in source form.
- Not every Markdown renderer supports Mermaid — noted for anyone reading outside GitHub.

### Neutral
- Some diagram types (detailed network topology, rich infographics) would need a different tool. None are required for this project.

## Revisit when

- A diagram genuinely cannot be expressed in Mermaid → produce that one in a drawing tool and commit both source and export, documenting the exception.
- The documentation moves to a platform without Mermaid support.
