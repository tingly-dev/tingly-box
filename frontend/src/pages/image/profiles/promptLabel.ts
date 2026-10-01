// What to call a prompt (or a profile made from one) that nobody has named:
// its own opening words, so the tab says what the prompt is about instead of
// "Prompt 4". A name the user types always wins over this.
export const deriveLabel = (text: string, max = 10): string => {
    const first = text.trim().split(/[，。,.!?！？；;：:\n]/)[0]?.trim() ?? '';
    return first.length > max ? `${first.slice(0, max)}…` : first;
};
