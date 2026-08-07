# Notes for agents

- never bypass commit signing - if you're blocked stopped and wait for the user
- never bypass git hooks

# Versioning and Changelog

This project uses semantic versioning and generates a changelog based on conventional commits.

It's very important to keep a good commit hygiene so we don't mess this up:

- new user facing features should start with `feat:`
- bug fixes should start with `fix:`

To avoid non user facing product changes from impacting the versioning and changelog:

- any updates to docs / website should start with `docs:`
- anything else that is not product related or user facing should generally be considered a `chore:`

When working in PRs the title should be in conventional commits form, because
PRs get squashed and their title becomes the commit name. Keep it under 52 chars
total.
