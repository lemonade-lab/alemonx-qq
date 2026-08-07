// Parsing of the plugin output convention: a line prefixed with "✓ ", "! " or
// "? " (marker + single space) carries a status; everything else is plain
// text. A leading backslash escapes the marker. Mirrors the host's
// SetupPluginResult parser so the web UI and the management UI agree.

export type StatusLineKind = 'ok' | 'fail' | 'warn' | 'plain'
export type StatusLine = { kind: StatusLineKind; text: string }

const STATUS_PREFIX = /^([✓!?]) /
const STATUS_ESCAPE = /^\\([✓!?]) /

export function parseStatusLine(raw: string): StatusLine {
  const line = raw.endsWith('\r') ? raw.slice(0, -1) : raw
  const escape = STATUS_ESCAPE.exec(line)
  if (escape) return { kind: 'plain', text: line.slice(1) }
  const match = STATUS_PREFIX.exec(line)
  if (match) {
    const kind =
      match[1] === '✓' ? 'ok' : match[1] === '!' ? 'fail' : 'warn'
    return { kind, text: line.slice(2) }
  }
  return { kind: 'plain', text: line }
}

export function splitStatusLines(output: string): StatusLine[] {
  return output.split(/\r?\n/).map(parseStatusLine)
}
