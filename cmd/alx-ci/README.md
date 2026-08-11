# alx-ci

`alx-ci` is the Go-only replacement for the former repository Python and
shell scripts. It validates the manifest and reviewed evidence, writes release
versions, prepares platform manifests, and performs E2E evidence checks.

E2E runners must provide a direct command as JSON, never a shell expression:

```json
["/opt/alx-e2e/lucky-check", "--platform", "linux-amd64"]
```

Set that JSON value in the matching GitHub Actions variable named
`LUCKYLILLIA_E2E_COMMAND_JSON_<PLATFORM>` or
`NAPCAT_E2E_COMMAND_JSON_<PLATFORM>`. The CI tool invokes the first item as an
executable with the remaining items as literal arguments; it does not use a
shell or expand command text.
