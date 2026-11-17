import {Box, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {Speed as IconLimits} from '@/components/icons';
import NodeTooltip from '@/components/nodes/NodeTooltip';
import type {Limits} from './LimitsEditor';
import {summarizeLimits} from './limitsSummary';

interface LimitsBadgeProps {
    /** This rule's share of the account (Service.Limits). */
    serviceLimits?: Limits | null;
    /** The account's own caps (Provider.Limits). */
    accountLimits?: Limits | null;
}

/**
 * Resting-state marker for a routing-graph service node: "this one has limits".
 * The editor lives behind the hover action bar, so without this the caps are
 * invisible until the pointer is on the node. Renders nothing when neither the
 * service nor its account is limited; the tooltip spells out the numbers.
 */
export default function LimitsBadge({serviceLimits, accountLimits}: LimitsBadgeProps) {
    const {t} = useTranslation();
    const service = summarizeLimits(serviceLimits, t);
    const account = summarizeLimits(accountLimits, t);
    if (!service.length && !account.length) return null;

    const tooltip = (
        <Box sx={{whiteSpace: 'pre-line'}} data-testid="limits-badge-tooltip">
            {service.length > 0 && <Typography variant="subtitle2" component="div">{t('limits.summary.service')}</Typography>}
            {service.map((line) => <Box key={`s-${line}`}>{line}</Box>)}
            {account.length > 0 && <Typography variant="subtitle2" component="div" sx={{mt: service.length ? 0.5 : 0}}>{t('limits.summary.account')}</Typography>}
            {account.map((line) => <Box key={`a-${line}`}>{line}</Box>)}
        </Box>
    );

    return (
        // The badge sits inside the node's identity tooltip (provider / model / style).
        // mouseover bubbles, so without this the outer tooltip would open on top of ours
        // and hide the numbers: keep the pointer's mouseover from reaching it.
        <Box component="span" sx={{display: 'inline-flex', lineHeight: 0}} onMouseOver={(e) => e.stopPropagation()}>
            <NodeTooltip title={tooltip} placement="top">
                <Box
                    component="span"
                    role="img"
                    aria-label={t('limits.summary.indicator')}
                    data-testid="limits-badge"
                    sx={{display: 'inline-flex', lineHeight: 0, color: service.length ? 'primary.main' : 'text.secondary'}}
                >
                    <IconLimits sx={{fontSize: 16}}/>
                </Box>
            </NodeTooltip>
        </Box>
    );
}
