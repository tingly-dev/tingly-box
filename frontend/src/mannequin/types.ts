// The two shapes the library shares with whoever draws it. Structural on
// purpose: a canvas point is `{x, y}` and a canvas is `{width, height}`, and
// naming them here keeps the library from importing the app.
export interface Point { x: number; y: number }
export interface Size { width: number; height: number }
