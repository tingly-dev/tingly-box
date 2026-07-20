import type { CSSProperties } from 'react';
import type { SxProps, Theme } from '@mui/material/styles';

/**
 * Render-stable empty defaults.
 *
 * Object/array literals used as default prop values (e.g. `sx = {}`) are
 * re-created on every render, which breaks referential equality and causes
 * unnecessary child re-renders. These constants are hoisted to module scope so
 * a single stable reference is shared across the whole app.
 */

/** Shared empty `sx` for `SxProps<Theme>`-typed props. */
export const EMPTY_SX: SxProps<Theme> = {};

/** Shared empty `sx` for `React.CSSProperties`-typed props. */
export const EMPTY_STYLE: CSSProperties = {};
