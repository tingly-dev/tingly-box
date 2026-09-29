/** First name not in `taken` of the form `<base><sep>copy`, then `…<sep>2`, `…<sep>3`, … */
export function nextFreeName(base: string, taken: readonly string[], sep = ' '): string {
    const first = `${base}${sep}copy`;
    let candidate = first;
    for (let i = 2; taken.includes(candidate); i++) candidate = `${first}${sep}${i}`;
    return candidate;
}
