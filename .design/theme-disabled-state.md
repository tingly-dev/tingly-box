# Disabled-state colors across themes

## Symptom

Disabled buttons (e.g. the Quick Proxy "Save" button on the System page)
were effectively unreadable — worst in the `dark` and `ds` themes, but the
same bug existed in every theme.

## Root causes (two, stacking)

1. **`primaryGradientButton` (`theme/components/buttonVariants.ts`) matched
   disabled buttons too.** It's a `variants` entry keyed on
   `{ variant: 'contained', color: 'primary' }`. MUI's `variants` matcher
   doesn't consider `disabled` unless you tell it to, so this rule also won
   against a disabled primary contained Button and painted the full brand
   gradient over it — overriding MUI's own `.Mui-disabled` style (which
   would otherwise fall back to `action.disabledBackground`/`action.disabled`).
   A disabled "Save" button looked pixel-identical to an enabled one, just
   with (also-broken) near-invisible label text.

2. **Every theme's `palette.action.disabled` held a background-tint value,
   not a foreground one.** MUI uses `action.disabled` as the *text/icon*
   color for a disabled Button, IconButton, Chip, etc, and a *separate*
   token, `action.disabledBackground`, for a disabled contained Button's
   fill. Every theme in this app set `action.disabled` to a very faint tint
   (e.g. ds: `rgba(85, 128, 250, 0.06)`, dark: `rgba(255, 255, 255, 0.05)`,
   light: a near-white `#f1f5f9`) — clearly intended as a subtle background
   wash, matching the naming pattern of `hover`/`selected`. None of the
   themes set `disabledBackground` at all, so it fell back to MUI's
   generic per-mode default. Net effect: disabled button *text* rendered at
   ~5% opacity against its background — invisible regardless of what the
   background was doing.

## Fix

- `buttonVariants.ts`: added `disabled: false` to the variant's prop
  matcher, so a disabled contained-primary Button now correctly falls
  through to MUI's own disabled styling instead of the gradient.
- Every palette (`theme/palettes/*.ts`): `action.disabled` now reuses that
  theme's own `text.disabled` value (already a well-tuned, legible color —
  nothing new to design). The old `action.disabled` value moved, unchanged
  in spirit, to the new `action.disabledBackground` field, which is what
  MUI actually reads for a disabled contained Button's fill. `ds` and
  `dark` also got a slightly stronger `disabledBackground` (closer to MUI's
  own ~0.12 default) since their originals were the faintest of the five.
- `components/ConnectProviderDialog.tsx`: its dialog-scrollbar thumb had
  been borrowing `action.disabled` purely because it happened to be a
  convenient faint color — now that the token carries a legible foreground
  value, that borrowed use would look far too bold. Switched it to
  `divider`, which is what it was actually going for.

## Where this lives

- `frontend/src/theme/components/buttonVariants.ts`
- `frontend/src/theme/palettes/{light,dark,claude,sunlit,ds}.ts`
- `frontend/src/components/ConnectProviderDialog.tsx`
