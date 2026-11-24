import {useEffect, useState} from 'react';
import {Box, LinearProgress} from '@mui/material';
import {useTheme} from '@mui/material/styles';
import {useTranslation} from 'react-i18next';
import {Fact, RailCard, type SessionLaunch, type Thread} from '@tingly/projects';
import {quotaColor, resetsIn} from '@/components/claudeCode/RouteStatusLine';
import {profileApi, type ClaudeCodeRoute} from '@/services/profileApi';
import { radius } from '@tingly/ui/theme/radius';

// QuotaCard is what the terminal status line shows after the model, as a card
// in the Projects info rail: the provider the thread's launch routes to, and
// what is left of its quota windows. Projects knows nothing of quota; it hands
// the card the thread and how its next turn launches (SessionSlots.railCards).
// The data is the route a folder session's status line reads, asked again after
// each turn. No route, or a provider without quota, is no card.
const QuotaCard = ({thread, launch}: {thread: Thread; launch: SessionLaunch}) => {
    const {t} = useTranslation();
    const theme = useTheme();
    // A reading belongs to the thread and launch it was asked for: the panel
    // stays mounted as the selection moves, and must not show another thread's.
    const key = `${thread.id}|${launch.profile}|${launch.model}`;
    const [fetched, setFetched] = useState<{key: string; route?: ClaudeCodeRoute}>();
    useEffect(() => {
        let live = true;
        profileApi.getClaudeCodeRoute({profile: launch.profile, tier: launch.model, model: thread.reported_model})
            .then((route) => live && setFetched({key, route}))
            .catch(() => live && setFetched({key}));
        return () => {
            live = false;
        };
    }, [key, launch.profile, launch.model, thread.reported_model, thread.last_turn_at]);

    const route = fetched?.key === key ? fetched.route : undefined;
    if (!route || route.quota.length === 0) return null;
    const routed = route.provider_name && (route.provider_model ? `${route.provider_model} @ ${route.provider_name}` : route.provider_name);
    return (
        <RailCard id="quota" title={t('claudeSession.quotaTitle')}>
            {routed && <Fact label={t('claudeSession.quotaProvider')} title={t('claudeSession.quotaProviderTitle')}>{routed}</Fact>}
            {route.quota.map((q, i) => {
                const label = q.balance ? t('claudeSession.statusBalance') : t(`claudeSession.quotaWindow.${q.type}`, {defaultValue: q.type});
                const reset = resetsIn(q.resets_at);
                return (
                    <Box key={`${q.type}-${i}`} sx={{py: 0.25}}>
                        <Fact label={label} title={reset ? t('claudeSession.statusResets', {reset}) : undefined}>{q.text}</Fact>
                        {!q.balance && (
                            <LinearProgress variant="determinate" value={Math.max(0, 100 - q.used_percent)} aria-label={label}
                                sx={{height: 4, borderRadius: radius.pill, mt: 0.25, '& .MuiLinearProgress-bar': {bgcolor: quotaColor(theme, q)}}}/>
                        )}
                    </Box>
                );
            })}
        </RailCard>
    );
};

export default QuotaCard;
