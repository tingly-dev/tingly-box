import { useEffect, useState } from 'react';
import { api } from '@/services/api';
import { normalizePoints } from '@/components/flags/RecordingV2Control';

// Must mirror PluginFeatures' view of "non-chat scenario" (no Thinking / Vision Proxy).
const NON_CHAT_SCENARIOS = new Set(['embed', 'imagegen', 'decisions']);

/**
 * Whether any scenario-level plugin deviates from its default — independent of
 * whether PluginFeatures is mounted (the connection card may load folded), so
 * the folded card can surface the plugins row only when something is on.
 * Reads the same three sources PluginFeatures shows.
 */
export const usePluginsActive = (scenario: string): boolean => {
    const [active, setActive] = useState(false);
    useEffect(() => {
        let mounted = true;
        const base = scenario.includes(':') ? scenario.split(':')[0] : scenario;
        const isChatShaped = !NON_CHAT_SCENARIOS.has(base);
        (async () => {
            try {
                const [effortR, recR, cfgR] = await Promise.all([
                    api.getScenarioStringFlag(scenario, 'thinking_effort'),
                    api.getScenarioStringFlag(scenario, 'recording_v2'),
                    api.getScenarioConfig(scenario),
                ]);
                if (!mounted) return;
                const effort = effortR?.success ? effortR.data?.value ?? '' : '';
                const record = recR?.success ? normalizePoints(recR.data?.value ?? '') : [];
                const ext = cfgR?.data?.extensions || cfgR?.data?.Extensions;
                const visionOn = !!(ext?.vision_proxy_service?.provider && ext?.vision_proxy_service?.model);
                setActive(Boolean((isChatShaped && effort) || record.length > 0 || (isChatShaped && visionOn)));
            } catch {
                if (mounted) setActive(false);
            }
        })();
        return () => { mounted = false; };
    }, [scenario]);
    return active;
};
