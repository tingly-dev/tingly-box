import {Box, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {Speed as IconLimits} from '@/components/icons';
import NodeTooltip from '@/components/nodes/NodeTooltip';
import type {Limits} from './LimitsEditor';
import {summarizeLimits} from './limitsSummary';

/**
 * Class of a badge that has nothing to show yet: the node's wrapper reveals it
 * on hover, like its other controls (see ServiceNode).
 */
export const LIMITS_QUIET_CLASS = 'limits-quiet';

interface LimitsBadgeProps {
    /** This rule's share of the account (Service.Limits). */
    serviceLimits?: Limits | null;
    /** The account's own caps (Provider.Limits). */
    accountLimits?: Limits | null;
    /** Opens the editor. Without it the badge only informs, and only when something is limited. */
    onClick?: () => void;
}

/**
 * The limits control of a routing-graph service node, a sibling of the quota
 * ring: its own small icon, hoverable for the numbers, clickable to edit.
 * Limited (rule share and/or account): always visible, blue when the rule's own
 * share is limited and grey when only the account is. Unlimited but editable:
 * a quiet icon the node reveals on hover, so setting a first limit is one click
 * away without a mark on every node.
 */
export default function LimitsBadge({serviceLimits, accountLimits, onClick}: LimitsBadgeProps) {
    const {t} = useTranslation();
    const service = summarizeLimits(serviceLimits, t);
    const account = summarizeLimits(accountLimits, t);
    const limited = service.length > 0 || account.length > 0;
    if (!limited && !onClick) return null;

    const tooltip = limited ? (
        <Box sx={{whiteSpace: 'pre-line'}} data-testid="limits-badge-tooltip">
            {service.length > 0 && <Typography variant="subtitle2" component="div">{t('limits.summary.service')}</Typography>}
            {service.map((line) => <Box key={`s-${line}`}>{line}</Box>)}
            {account.length > 0 && <Typography variant="subtitle2" component="div" sx={{mt: service.length ? 0.5 : 0}}>{t('limits.summary.account')}</Typography>}
            {account.map((line) => <Box key={`a-${line}`}>{line}</Box>)}
            {onClick && <Box sx={{mt: 0.5, opacity: 0.7}}>{t('limits.summary.clickToEdit')}</Box>}
        </Box>
    ) : (
        <Box data-testid="limits-badge-tooltip">{t('limits.buttonTooltip')}</Box>
    );

    const activate = (e: {stopPropagation: () => void}) => {
        e.stopPropagation(); // the node itself opens the provider editor
        onClick?.();
    };

    return (
        // The badge sits inside the node's identity tooltip (provider / model / style).
        // mouseover bubbles, so without this the outer tooltip would open on top of ours
        // and hide the numbers: keep the pointer's mouseover from reaching it.
        <Box component="span" className={limited ? undefined : LIMITS_QUIET_CLASS}
             sx={{display: 'inline-flex', lineHeight: 0}} onMouseOver={(e) => e.stopPropagation()}>
            <NodeTooltip title={tooltip} placement="top">
                <Box
                    component="span"
                    role={onClick ? 'button' : 'img'}
                    tabIndex={onClick ? 0 : undefined}
                    aria-label={limited ? t('limits.summary.indicator') : t('limits.button')}
                    data-testid="limits-badge"
                    onClick={onClick ? activate : undefined}
                    onKeyDown={onClick ? (e) => {
                        if (e.key !== 'Enter' && e.key !== ' ') return;
                        e.preventDefault();
                        activate(e);
                    } : undefined}
                    sx={{
                        display: 'inline-flex', lineHeight: 0, p: '2px', borderRadius: '50%',
                        color: !limited ? 'text.disabled' : service.length ? 'primary.main' : 'text.secondary',
                        cursor: onClick ? 'pointer' : 'default',
                        '&:hover': onClick ? {bgcolor: 'action.hover'} : undefined,
                    }}
                >
                    <IconLimits sx={{fontSize: 16}}/>
                </Box>
            </NodeTooltip>
        </Box>
    );
}
