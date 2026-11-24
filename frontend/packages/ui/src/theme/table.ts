import { radius } from './radius';

// A table has one of two shapes (`.design/visual-system.md` §2.4 rule 8):
//
// - Inside a card, next to a title or other content: the table sits in its own
//   frame — this sx on the TableContainer. Credentials, Bots, Team keys.
// - As the card itself (a padding-0 card whose body is a header strip and the
//   table): no frame of its own; the card's border is the frame. Dashboard and
//   usage tables, log viewers.
//
// The frame is a divider-coloured border on the card corners. The card's
// surface shows through, and the last row drops its rule so it doesn't double
// the frame's bottom edge.
export const tableFrameSx = {
    border: 1,
    borderColor: 'divider',
    borderRadius: radius.card,
    '& .MuiTableBody-root .MuiTableRow-root:last-of-type > .MuiTableCell-root': { borderBottom: 0 },
} as const;
