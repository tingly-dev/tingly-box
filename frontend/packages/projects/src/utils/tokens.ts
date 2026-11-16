/** Same estimate as the backend (internal/knowledge.EstimateTokens): ~4 ASCII
 * characters per token, one token per non-ASCII character. */
export function estimateTokens(s: string): number {
    let ascii = 0;
    let other = 0;
    for (const ch of s) {
        if (ch.charCodeAt(0) < 0x80) ascii++;
        else other++;
    }
    return Math.floor((ascii + 3) / 4) + other;
}
