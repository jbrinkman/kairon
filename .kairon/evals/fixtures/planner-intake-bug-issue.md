# Export prints the same run twice when an issue number is repeated

Issue #433

## Description

When the same issue number is given more than once to the export command, that issue's runs show up
more than once in the output.

## Reproduction

1. Make sure the run history directory contains `issue-7.json` with two records.
2. Run:

   ```bash
   kairon export --format csv --issues 7,7
   ```

3. Look at the output.

## Current behavior

The CSV contains four data rows: the two records for issue 7, then the same two records again.

## Expected behavior

The CSV contains two data rows, one per record for issue 7. Each run appears once.
