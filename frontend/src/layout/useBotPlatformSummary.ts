import { useEffect, useState } from 'react';
import { api } from '@/services/api';

type PlatformSummary = Record<string, { active: number; total: number }>;

export function useBotPlatformSummary(enabled: boolean): PlatformSummary {
    const [summary, setSummary] = useState<PlatformSummary>({});

    useEffect(() => {
        if (!enabled) return;
        let cancelled = false;
        api.getImBotSettingsList()
            .then(data => {
                if (cancelled || !data?.success || !Array.isArray(data.settings)) return;
                const map: PlatformSummary = {};
                for (const bot of data.settings) {
                    if (!bot?.platform) continue;
                    const slot = map[bot.platform] ?? (map[bot.platform] = { active: 0, total: 0 });
                    slot.total++;
                    if (bot.enabled) slot.active++;
                }
                setSummary(map);
            })
            .catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [enabled]);

    return summary;
}
