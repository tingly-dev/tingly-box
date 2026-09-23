# @tingly/vision

Browser-native image/video processing that the Image Playground's UI is built
on top of: canvas sketch layers, edit masks (the alpha channel an
/images/edits request carries), background matting (chroma-key + auto
background detection), sticker-sheet slicing, GIF encoding (LZW + palette
quantization), video-to-GIF conversion, and zip archive creation.

It depends on nothing but the browser's own Canvas2D/Blob/File APIs — no
React, no MUI, no i18n, no app services. Every module is pure enough to be
unit-tested standalone (see the `*.test.ts` files next to each one).

Consumed as TypeScript source (`exports` points at `src/index.ts`); the app's
bundler compiles it, so there is no build step.
