# Glossa conformance cases

Cases the Unicode suite doesn't cover, which every Glossa MessageFormat
implementation must pass as well:

- `mf1-to-mf2.json`: ICU MessageFormat 1 source → canonical MF2 data model.
- `arguments.json`: message → extracted arguments (name, type, selector cases, markup).
- `compat.json`: (source, translation) pairs → structural compatibility findings.
- `runtime-format.json`: precompiled data model + values + locale → formatted output,
  for runtimes that interpret the data model without a parser.

Each file follows the shape documented at its top (`"$comment"`). Cases are
added whenever a bug is found in any implementation, so the fix is proven
everywhere.
