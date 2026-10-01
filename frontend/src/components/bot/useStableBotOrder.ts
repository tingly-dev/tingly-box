import type {BotSettings} from '@/types/bot';
import {useMemo, useState} from 'react';

// useStableBotOrder lists live bots first and off bots after them, but takes
// that order ONCE, from the first non-empty load. After that, flipping a
// card's switch must not move the card out from under the pointer
// (ux-principles #12); bots added later go to the end. Shared by the Remote
// Control and IM Notify pages, which each define "live" for their purpose.
export function useStableBotOrder(bots: BotSettings[], loading: boolean, isLive: (bot: BotSettings) => boolean): BotSettings[] {
    const [order, setOrder] = useState<string[] | null>(null);
    if (order === null && !loading && bots.length > 0) {
        setOrder([...bots].sort((a, b) => Number(isLive(b)) - Number(isLive(a))).map((bot) => bot.uuid || ''));
    }
    return useMemo(() => {
        const rank = (bot: BotSettings) => {
            const i = order ? order.indexOf(bot.uuid || '') : -1;
            return i < 0 ? Number.MAX_SAFE_INTEGER : i;
        };
        return [...bots].sort((a, b) => rank(a) - rank(b));
    }, [bots, order]);
}
