/** How a Team Key is shown until the user reveals it: prefix … last four. */
export const maskToken = (token: string): string => {
    if (!token) return '';
    if (token.length <= 16) return `${token.slice(0, 8)}...${token.slice(-4)}`;
    return `${token.slice(0, 12)}...${token.slice(-4)}`;
};
