# Add a JSON Lines exporter

Issue #412

## Description

We can export run history as CSV and Markdown. Please add a JSON Lines exporter so people can pipe run
history into `jq` and log tooling. Each finished agent run should become one JSON object per line, using the
same fields the other exporters already show.

It should be selectable the same way as the other formats.

## Notes

Reviewers asked that it be well tested.
