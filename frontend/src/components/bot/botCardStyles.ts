// Shared shell + status-chip styling for BotCard and RemoteAgentBotCard —
// the two "twin" per-bot cards (see RemoteAgentBotCard's own comment). Both
// used to hand-roll an identical `styled(Card)` with a dashed border,
// grayscale filter, and a diagonal-stripe overlay for the disabled state —
// a bespoke treatment found nowhere else in the app. The established
// pattern for a resource row (ApiKeyTable, RulesPage, ToolCard) is a plain
// bordered container plus an explicit On/Off chip; extracted here once so
// both cards render identically and can't drift apart again.
export const BOT_CARD_SX = {
    bgcolor: 'background.paper',
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 2,
    boxShadow: 'none',
} as const;

export const statusChipSx = { height: 22, minWidth: 40 } as const;
