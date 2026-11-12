// Recursive parity type: every zh locale file is annotated as Dict<typeof
// its en counterpart>, so a key present in en but missing (or mistranslated
// into a non-string) in zh fails typecheck. en files stay unannotated —
// they are the key source of truth.
export type Dict<T> = { [K in keyof T]: T[K] extends string ? string : Dict<T[K]> };
