// Component-level colour tokens: the values each theme fills in so that one
// factory (components/factory.ts) can style every MUI component the same way
// in every theme. A theme is its MUI palette plus one of these tables; the
// component overrides themselves are written once, never per theme.
//
// Colours only — sizes, radii and weights are shared and live in the factory.
// Optional fields are surfaces a theme leaves to MUI's default.

export interface ThemeTokens {
  /** Focus ring, focused label, tab indicator. */
  accent: string;
  /** Text inside inputs, tooltips and the app bar. */
  text: string;
  /** Input labels and the select chevron. */
  textMuted: string;
  /** Form helper text. */
  textHelper: string;
  /** Input placeholders. */
  placeholder: string;
  /** Error outline and error helper text. */
  error: string;

  surface: {
    card: string;
    appBar: string;
    menu: string;
    popover: string;
    dialog: string;
    tooltip: string;
    /** Drawer paper; unset keeps MUI's `background.paper`. */
    drawer?: string;
    /** Every Paper; unset keeps MUI's `background.paper`. */
    paper?: string;
  };

  border: {
    card: string;
    /** App bar bottom and drawer edge. */
    chrome: string;
    menu: string;
    tooltip: string;
    divider: string;
    /** Dialog and popover outlines; unset draws none. */
    dialog?: string;
    popover?: string;
  };

  shadow: {
    menu: string;
    tooltip: string;
  };

  input: {
    bg: string;
    hoverBg?: string;
    border: string;
    hoverBorder: string;
    disabledBorder: string;
    disabledBg?: string;
    filledBg: string;
    filledHoverBg: string;
    filledFocusBg: string;
  };

  button: {
    outlinedBorder: string;
    outlinedText: string;
    outlinedHoverBorder: string;
    outlinedHoverBg: string;
    /** Primary contained button gradient: from, to, hover from, hover to. */
    gradient: readonly [string, string, string, string];
  };

  menuItem: {
    hover: string;
    selected: string;
    selectedHover: string;
  };

  table?: {
    headBg?: string;
    cellBorder?: string;
    rowHover?: string;
  };
}
