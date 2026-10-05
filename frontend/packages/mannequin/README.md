# @tingly/mannequin

A rigged, three-dimensional artist's mannequin: sixteen joints with forward
kinematics, joint limits (`constrainFigure`), two builds (female / male, `setFigureBuild`)
sharing one pose library of forty-four presets, a per-figure perspective camera, hit testing and handles for an
editor, a MediaPipe landmark retarget, and a WebGL renderer.

It depends on nothing but `three`. Points and sizes are structural
(`{x, y}` / `{width, height}`), so any canvas can host it.

- `poses/library.ts` — the pose library. Add a pose there and nothing else changes.
- `body.ts` — the model: every size per build, the solid list the renderer draws, the palette.
- `build.ts` — changing a figure's build without changing its pose.
- `.design/mannequin-standard.md` in the repo root records why the model is shaped as it is.

Consumed as TypeScript source (`exports` points at `src/index.ts`); the app's
bundler compiles it, so there is no build step.
