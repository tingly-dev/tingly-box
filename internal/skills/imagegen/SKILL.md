---
name: imagegen
description: Generate new images from a text prompt, or edit existing images (restyle, change background, add/remove objects, combine references, masked inpainting), through the user's Tingly-Box image endpoint. Use whenever the user asks to create, draw, render, design, mock up, modify, retouch or iterate on an image, icon, logo, illustration or picture file — including follow-up edits of an image generated earlier in the conversation.
---

# ImageGen (Tingly-Box)

One script covers both directions, so generate → edit → edit again stays in this skill:

```bash
python3 <skill-dir>/scripts/image.py models
python3 <skill-dir>/scripts/image.py generate --prompt "..." [--size 1024x1024] [--n 1]
python3 <skill-dir>/scripts/image.py edit --prompt "..." --image in.png [--image ref.png] [--mask mask.png]
```

`<skill-dir>` is the directory containing this file. Python 3 stdlib only.

## Endpoint, token, model

The script reuses what is already configured, in this order, and guesses nothing else:

1. `--base-url` / `--token` / `--model`
2. `TINGLY_IMAGE_BASE_URL` / `TINGLY_IMAGE_TOKEN` / `TINGLY_IMAGE_MODEL`
3. The agent's own Tingly-Box connection (`ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`,
   or `OPENAI_BASE_URL` + `OPENAI_API_KEY`) when it points at `/tingly/<scenario>` — same
   token, the scenario is swapped for `imagegen`. If exactly one model is served there it is
   used; otherwise the user picks.

When something is missing or rejected the script exits with status **2** and prints a line
starting with `NEED_INPUT:` naming what is needed. Then:

- **Ask the user** for that value (endpoint such as `http://localhost:12580`, their
  Tingly-Box model token, or which model from the listed ones). Do not hunt for it in config
  files, and do not invent a default.
- Re-run with the matching flag. Mention they can export `TINGLY_IMAGE_*` to skip the
  question next time; do not write it into their shell profile yourself.

## Workflow

1. Turn the request into a concrete prompt: subject, style, composition, colors, text to
   render (quoted), what must stay unchanged (for edits).
2. Run `generate` or `edit`. Output goes to `./generated-images/` unless the user named a
   place (`--out`). Image calls can take a minute; the default timeout is 300 s.
3. Report the saved path(s) from the JSON output and show/open the image if the client can.
   Keep the path — a follow-up "make it bluer" is an `edit --image <that path>`.

### Options

| Flag | Values |
|------|--------|
| `--size` | `1024x1024`, `1536x1024`, `1024x1536`, `auto` (provider-dependent) |
| `--quality` | `low`, `medium`, `high`, `auto` |
| `--background` | `transparent`, `opaque`, `auto` (transparent needs png/webp) |
| `--output-format` | `png` (default), `jpeg`, `webp` |
| `--n` | number of variants |
| `--image` | input image; repeat for extra references (edit only) |
| `--mask` | PNG, same size as the first image; transparent pixels = area to change (edit only) |

Omit flags the user didn't ask for — the provider defaults are usually right. If the provider
rejects an option, drop it and retry once rather than looping.
