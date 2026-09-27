# Vendored bubbletea

This is `github.com/charmbracelet/bubbletea` v1.3.10, vendored so ircgo can
carry a small input-parser fix that upstream hasn't released. It is wired up
with a `replace` directive in the top-level `go.mod`:

```
replace github.com/charmbracelet/bubbletea => ./third_party/bubbletea
```

## Patches

### `key.go` — don't leak split SGR mouse sequences as text

**Problem:** with mouse all-motion tracking on, bursts of SGR motion events
(`\x1b[<35;80;20M` etc.) routinely straddle bubbletea's 256-byte stdin reads.
When a read ended mid-sequence (e.g. `\x1b[<3`), `detectOneMsg` fell through
the mouse branch (the regex needs the full sequence) and emitted the partial
as `Alt+[` followed by the remainder as literal runes. The raw escape
sequences landed in the chat input — and could even be sent as a message.

**Fix:** in `detectOneMsg`, right after the mouse branch: if more input may be
coming (`canHaveMoreData`) and the buffer is a CSI introducer (`\x1b[`,
optional SGR `<`) followed only by parameter bytes (digits/semicolons) with
no final byte, return `(0, nil)` to wait for the rest of the sequence instead
of parsing a partial. Complete sequences and short reads (real boundaries)
are unaffected.

**Tests:** `sgr_burst_test.go` — `TestBurstMotionParsing` (60-event burst
across 256-byte reads, zero leaks) and `TestPartialSGRMouseWaits`.

## Maintenance

- To re-vendor a newer upstream: copy the new version over this directory,
  re-apply the `key.go` patch above, and re-run the tests:
  `cd third_party/bubbletea && go test ./...`
- If upstream ever fixes this, the whole directory and the `replace`
  directive can go away.
