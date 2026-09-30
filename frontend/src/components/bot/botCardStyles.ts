// Shell styling for RemoteAgentBotCard. An inactive card (purpose off, or
// the bot itself disabled) used to get a diagonal hatch overlay; that read
// as "broken / greyed-out screenshot" and made the off card the noisiest
// thing on the page. Off is a normal, quiet state: the card drops its paper
// background (so it recedes into the page) and the card itself collapses
// its content — see RemoteAgentBotCard.
export const botCardSx = (active: boolean) => ({
    position: 'relative' as const,
    bgcolor: active ? 'background.paper' : 'transparent',
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 2,
    boxShadow: 'none',
    transition: 'background-color 0.18s ease-out',
});
