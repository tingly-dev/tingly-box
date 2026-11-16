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

## References are files, previews are blob URLs

A reference (`ReferenceImage`) is its `file`; `previewUrl` is only how a page
shows it. Every new reference gets `URL.createObjectURL(file)` (upload, paste
from an output, library pick, sketch, run re-entry) instead of a base64 copy.
Because a blob: URL does not outlive the page:

- `imageProfileStore.reviveProfile` rebuilds `previewUrl` from `file` on load.
  Profiles saved by older builds carry a base64 copy; those are rewritten once
  without it (new object → the store saves it). The rest are fixed in place,
  like `reviveSessionBlobs`, so a load does not rewrite every profile.
- `GenerationRun.sourceBlobs` keeps the files behind `sourceImages`;
  `reviveSessionBlobs` rebuilds `sourceImages` from them. Old runs without
  `sourceBlobs` keep their data URLs.
- `SketchResult` no longer carries a `previewUrl`; the canvas is not encoded a
  second time.

Measured on 5 profiles × 3 × 1024px references: the boot-time profile read
went from ~300 ms to ~220 ms and no longer leaves ~65 MB of base64 strings
resident on every page. The sidebar still reads the whole collection (files
are blob handles, so this is cheap); a split summary record is not worth it
until profiles carry far more than references.

## Library assets are files too

`ImageAsset.blob` holds the bytes; `src` is a blob: URL rebuilt on load by
`assetStore.reviveAsset`. Base64 assets from older builds are rewritten once
as blobs; URL-encoded data URLs and links are left alone.

Because the URL is new on every load, "is this picture already kept" can no
longer be a `src` comparison: `keepImage` compares bytes (size, type, then the
buffer) and a second click on the same picture while the first is running
returns the same asset. The lightbox's "In the library" check is still by `src`,
so it is accurate within a session and shows the keep button again after a
reload; pressing it finds the existing asset and adds nothing.

## Still open

- Sketch `layers.backdrop` is a data URL (only when a backdrop was loaded);
  small and rare, left alone.
