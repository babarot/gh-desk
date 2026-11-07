# ADR-055: The site is published with a release

## Status

Accepted
Partly replaced by ADR-106

## Context

The site at babarot.me/gh-desk/ is the playground (ADR-047, ADR-052), and with ADR-054 it gains the docs. GitHub Pages serves one site for the repository, so the two are published together: a publish of one alone takes the other down.

The playground was published on every push to main that touched its code. The docs say what gh-desk does and name the version they are of, and the reference pages are written from the code (ADR-051). Published from main, they would describe what is not released yet, under the number of the last release.

tagpr makes the release tag with the workflow's own token, and a tag pushed that way starts no other workflow, so a workflow on tag pushes would never run.

## Decision

- The playground and the docs are built and published together, as one site: the playground at the root, the docs under `docs/`, and the playground on the whole window at `play/` (ADR-052).
- The site is published when a release is made, from the commit released: the Release workflow calls the Pages workflow after tagpr makes the release. The playground, the docs and the version the docs name are then those of the gh-desk people install.
- The Pages workflow can also be run by hand on main, which publishes main; the github-pages environment refuses a deployment from any other branch. A release is published again by re-running the pages job of its Release run.
- The playground's page and the docs link to each other, and keep the theme the visitor picked on either.
- Rejected: publishing on every push to main. The docs would describe unreleased changes under the last release's number.
- Rejected: publishing the playground from main and the docs from the last release's tag. Both are one site and one publish, so the workflow would build two commits at once, and the playground would show what the docs do not describe.

## Consequences

- A change merged to main reaches the site only at the next release; until then the site shows the last release.
- A change to the site alone (a typo in the docs, the playground's page) waits for a release too. Running the Pages workflow by hand on main publishes it sooner, together with whatever else main has that is not released.
- The Pages workflow does not run on its own, so a change that breaks the site's build is caught by the Test workflow's `site` job, before a release.
