# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Purpose

This is a learning/study repository for exploring authentication concepts. It is not a production codebase and has no shipping deadlines or external consumers.

## Working style

- Proceed feature-by-feature, one small step at a time. Do not race ahead implementing multiple features at once.
- After each feature (or sub-feature), stop and verify the behavior works before moving on.
- Prefer the minimal thing that demonstrates the concept being studied over a "complete" implementation.
- When in doubt about scope or next step, ask rather than assume — the point is learning, so surprising the user with a large diff defeats the purpose.
- Always explain the *why* alongside implementation instructions — what security or protocol purpose the code serves.

## Coding style

- Do not write code directly by default. Describe what to write and how; the user decides whether to implement it themselves or delegate to Claude.
- Only implement when the user explicitly says so ("お願い", "やって", "委ねます" etc.).

## Documentation

- After each implementation phase completes, proactively write a summary doc under `docs/` without waiting to be asked.
- Name docs with a numeric prefix: `docs/01-*.md`, `docs/02-*.md`, ...
- Use Mermaid for all diagrams. No ASCII art.

## User preferences

- Prefer config-as-code over admin UIs. Default to declarative files or scripted setup committed to the repo.
- Container runtime is Docker.
