# alx-ci

`alx-ci` is the Go-only replacement for the former repository Python and
shell scripts. It validates the manifest and reviewed evidence, writes release
versions, prepares platform manifests, and performs E2E evidence checks.

E2E runners must provide a direct command as JSON, never a shell expression:

```json
["/opt/alx-e2e/lucky-check", "--platform", "linux-amd64"]
```

Set that JSON value in the matching GitHub Actions variable named
`LUCKYLILLIA_E2E_COMMAND_JSON_<PLATFORM>`,
`SNOWLUMA_E2E_COMMAND_JSON_<PLATFORM>`, or
`NAPCAT_E2E_COMMAND_JSON_<PLATFORM>`. The CI tool invokes the first item as an
executable with the remaining items as literal arguments; it does not use a
shell or expand command text.

SnowLuma runs only on dedicated native hosts: `windows-amd64`, `linux-amd64`,
and `linux-arm64`. The E2E helper downloads and inspects the official full
package, then calls the configured host command with `SNOWLUMA_ARCHIVE`,
`SNOWLUMA_RELEASE_TAG`, `SNOWLUMA_PLATFORM`, and `SNOWLUMA_NATIVE_ADDON`.
That command must write `artifacts/snowluma-e2e-report.json` and prove the
install, preflight, start, WebUI, QQ detection, login-pending, Token read,
stop, update, and rollback stages. Do not write credentials or QR images into
that report.

The repository intentionally does not ship the host executable: it must be a
reviewed command installed on each self-hosted runner, because it controls QQ,
the Windows desktop session or Linux X11/Xvfb session, and ptrace permissions.
Before enabling the workflow, register the `snowluma-e2e` label on each target
runner and set the matching `SNOWLUMA_E2E_COMMAND_JSON_<PLATFORM>` repository
variable. Without both, `snowluma_e2e` is an unconfigured contract and not a
completed E2E validation.

The host command receives the validated release metadata through environment
variables and must write this shape, with every named stage set to `true` only
after observing the real plugin action:

```json
{
  "platform": "linux-amd64",
  "processModel": "managed-detached",
  "websocketRequired": true,
  "stages": {
    "install": true,
    "preflight": true,
    "start": true,
    "webUI": true,
    "qqProcess": true,
    "loginPending": true,
    "tokenRead": true,
    "stop": true,
    "update": true,
    "rollback": true
  }
}
```
