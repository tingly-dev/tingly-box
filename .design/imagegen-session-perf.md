# Image playground session: keep bytes, not base64

A long playground session (a dozen 2048px results) took ~20s to show its first
image after a reload and held ~0.6–1 GB of JS heap. Measured on a seeded
session in the mock app; the profile put ~3.6s in the IndexedDB read alone.

## Causes

- Results were kept as `b64_json` strings. Each is multi-MB; the whole session
  was structured-cloned out of IndexedDB on the main thread at mount, then
  copied again into a data URL per image.
- Run cards drew the full-size data URL in an `<img>`: every visible tile
  decoded the original (`ThumbImage` only covered the overview and strips).
- `GenerationRunCard` was not memoised and received fresh callbacks, so every
  panel render (a run landing, a keystroke-adjacent state change) re-rendered
  every card.

## Invariants

- A result that lands in the session is converted once by `toSessionImage`:
  base64 → `Blob`, rendered from a `blob:` URL (`url`), persisted as the `Blob`
  (`ImageResult.blob`). `resultSrc` is unchanged: it still prefers `url`.
  Imports do the same (`ImportedImage.blob`). Results that arrive as a
  provider URL are untouched.
- `blob:` URLs do not survive a page. `reviveSessionBlobs` rebuilds them
  **in place** after the IndexedDB load: the loaded objects are what
  `playgroundSession` believes it stored, so replacing them would make the
  next save rewrite every record.
- Old records with `b64_json` still render (`resultSrc` falls back to it) and
  are not migrated.
- Run-card tiles render through `ThumbImage` at `THUMB_EDGE_RUN`; the original
  is only decoded by the lightbox, downloads and "use as reference".
- Blob URLs are not revoked when a run is removed: copies of a run (profile
  save) share them. The bytes sit in the blob store, not the JS heap.

Not covered: reference images (`sourceImages`) are still data URLs.

## Thumbnails everywhere an image is shown small

Library grid, library picker, imported-image cards, the reference row, the
lightbox filmstrip and the sidebar's profile icon all drew the original (a
20px sidebar icon decoded a full reference image on every page). They render
through `ThumbImage` now.

## Still open

- `imageProfileStore` is read eagerly on every page (the sidebar lists
  profiles), including each profile's reference `File` **and** a base64
  `previewUrl` copy of the same image. A profile with a few large references
  costs hundreds of ms of main-thread IndexedDB deserialisation at boot.
  Fixing it means dropping the duplicate `previewUrl` (derive it from `file`),
  which also touches `GenerationRun.sourceImages` (data URLs by design: they
  must survive a reload and be re-sendable).
- Run `sourceImages` and library assets are still data URLs.
