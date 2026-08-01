# Content-addressed sealed journal batches

## Requirements

- Bind approval to exact journal content rather than a filesystem path.
- Preserve the existing no-active-writer, regular-file, private-mode, 256 MiB
  file, and 1 MiB event constraints.
- Return no private event until the complete artifact has passed validation and
  the expected digest matches.
- Bound one private response to 64 events and make pagination deterministic.
- Keep verification output identity-free and batch output explicitly private.
- Add no network client, credential, rotation daemon, database, or actuator.

## Architecture

`scanSealedJournal` owns `lstat → open → validate → shared flock → identity
recheck → full scan → final identity recheck → unlock`. A SHA-256 writer is
attached to the exact byte stream read by the scanner. The scan produces the
existing anomaly/time summary plus a content digest and optionally retains the
requested event range.

`VerifyJournal` calls the scanner without retaining events. `ReadJournalBatch`
requires a canonical expected digest, offset, and limit; it calls the same
scanner, compares the digest after EOF, validates the requested range, and only
then returns a typed batch. The CLI JSON-encodes the returned object after the
lock-owning function succeeds, so no partial private batch is emitted on a
validation or digest error.

## Failure behavior

- Active writer, symlink, non-regular file, unsafe mode, oversized file/event,
  malformed event, mixed domain, changed target, or digest mismatch: no stdout.
- Offset beyond the event count or limit outside `1..64`: reject.
- Duplicate IDs and timestamp regression remain counted rather than silently
  changed; an approval layer decides whether those exact bytes may be imported.
- Empty journals verify normally and allow one complete empty batch at offset 0.

## NFRs

- Memory: requested events only, at most 64 × 1 MiB plus decoder overhead.
- CPU/I/O: O(file size) per batch; document and benchmark before production.
- Confidentiality: verification summary contains no domain/resource/event IDs;
  batch output is private.
- Integrity: digest provides content equality only, not authenticity.
- Availability: offline command; failure cannot affect the controller or PVE.
