// The phone shell's layer over the app theme (`.design/mobile-ui.md`,
// "Configuration pages on a phone"). `MobileShell` marks the body with
// `data-shell="phone"` and installs these as global styles, so every page and
// dialog it shows gets phone-sized controls and sheet dialogs without a phone
// copy of itself — and nothing changes outside the phone shell.

import type { CSSObject } from '@mui/material/styles';
import { mobile, safeAreaBottom } from './mobile';

/** The body attribute the phone shell sets; the styles below are scoped to it. */
export const PHONE_SHELL_ATTR = 'data-shell';
export const PHONE_SHELL_VALUE = 'phone';

/** The visible height (shrinks with the on-screen keyboard), kept up to date by the shell. */
export const PHONE_VIEWPORT_HEIGHT = '--phone-viewport-height';
/** How far the visible area is scrolled down inside the layout viewport (iOS keyboard). */
export const PHONE_VIEWPORT_TOP = '--phone-viewport-top';
export const phoneViewportHeight = `var(${PHONE_VIEWPORT_HEIGHT}, 100dvh)`;

const tap = `${mobile.tapMin}px`;
// iOS zooms into any input whose text is under 16px.
const inputText = '16px';
// A sheet stops short of the top so it reads as a sheet over the page.
const sheetGap = '32px';
const sheetCorner = '24px';

export const phoneShellStyles: CSSObject = {
  [`body[${PHONE_SHELL_ATTR}="${PHONE_SHELL_VALUE}"]`]: {
    // Touch targets: 44px at least, whatever a page asked for.
    '& .MuiButton-root, & .MuiToggleButton-root, & .MuiTab-root, & .MuiMenuItem-root, & .MuiListItemButton-root': {
      minHeight: tap,
    },
    '& .MuiIconButton-root': {
      minWidth: tap,
      minHeight: tap,
    },
    // Fields: a 44px row and text that doesn't make iOS zoom.
    '& .MuiInputBase-root': {
      minHeight: tap,
      fontSize: inputText,
    },
    '& .MuiInputBase-input, & .MuiSelect-select': {
      fontSize: inputText,
    },

    // Dialogs are bottom sheets: full width, rounded top, at most the visible
    // height (so the keyboard never hides their actions), actions last.
    '& .MuiDialog-root': {
      top: `var(${PHONE_VIEWPORT_TOP}, 0px)`,
      bottom: 'auto',
      height: phoneViewportHeight,
    },
    '& .MuiDialog-container:not(.MuiDialog-scrollBody)': {
      alignItems: 'flex-end',
    },
    '& .MuiDialog-paper:not(.MuiDialog-paperFullScreen)': {
      margin: 0,
      width: '100%',
      maxWidth: '100%',
      maxHeight: `calc(100% - ${sheetGap})`,
      borderRadius: `${sheetCorner} ${sheetCorner} 0 0`,
    },
    '& .MuiDialog-paperFullScreen': {
      height: '100%',
      borderRadius: 0,
      paddingBottom: safeAreaBottom,
    },
    '& .MuiDialogActions-root': {
      flexWrap: 'wrap',
      paddingBottom: `calc(12px + ${safeAreaBottom})`,
    },
  },
};
