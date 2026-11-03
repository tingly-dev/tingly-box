import { Box, Tooltip } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { fontMono } from '../theme/fonts';
import { entityText, shortId, type EntityKind, type EntityParts } from '../utils/entityLabel';
import { CopyIconButton } from './CopyIconButton';

export interface EntityLabelProps extends EntityParts {
    /** Drives id shortening; defaults to `group` / `direct` from `group`. */
    kind?: EntityKind;
    /** Brand icon drawn before the platform name. */
    platformIcon?: ReactNode;
    /** `icon` hides the platform text (kept in the tooltip / aria-label) for rows already under a bot header. */
    platformAs?: 'text' | 'icon';
    /** Localised word for the group segment (`群` / `Group`). */
    groupLabel?: string;
    /** False when the referenced entity no longer exists: the stored id is shown with a muted marker. */
    resolved?: boolean;
    deletedLabel?: string;
    /** Show a copy button that always copies the full id (or `copyValue`). */
    copy?: boolean;
    /** What the copy button and the tooltip carry when it differs from the displayed id (e.g. the stored external id with its platform prefix). */
    copyValue?: string;
    copyLabel?: string;
    copiedLabel?: string;
    onCopied?: () => void;
    /** Tooltip on the id when there is no alias (e.g. "No alias set"). */
    unnamedHint?: string;
    sx?: SxProps<Theme>;
}

const Sep = () => (
    <Box component="span" aria-hidden sx={{ color: 'text.disabled', flexShrink: 0 }}>·</Box>
);

/**
 * `平台 · 群 · alias · id`. Presentational only: the caller resolves the
 * segments. Segments that are empty or do not apply are dropped with their
 * separator; the id takes the anchor position when there is no alias, so a
 * row never looks empty. The id may be shortened but the full id is always
 * reachable (tooltip, and the copy button when `copy` is set).
 */
export const EntityLabel = ({
    platform,
    group = false,
    alias,
    id,
    kind,
    platformIcon,
    platformAs = 'text',
    groupLabel = 'Group',
    resolved = true,
    deletedLabel = '(deleted)',
    copy = false,
    copyValue,
    copyLabel = 'Copy ID',
    copiedLabel = 'Copied!',
    onCopied,
    unnamedHint,
    sx,
}: EntityLabelProps) => {
    const entityKind: EntityKind = kind ?? (group ? 'group' : 'direct');
    const aliasText = alias?.trim() ?? '';
    const short = shortId(entityKind, id);
    const full = entityText({ platform, group, alias: aliasText, id }, { groupLabel, shorten: false, kind: entityKind });
    const idAnchor = aliasText === '';
    // With the platform drawn as a logo alone, the first segment after it needs no separator.
    const iconOnly = platformAs === 'icon' && Boolean(platformIcon);

    const idNode = (
        <Box
            component="span"
            sx={{
                fontFamily: fontMono,
                fontSize: '0.8125rem',
                color: idAnchor ? 'text.primary' : 'text.secondary',
                fontWeight: idAnchor ? 600 : 400,
                flexShrink: 0,
            }}
        >
            {short.text}
        </Box>
    );
    const fullId = copyValue ?? id;
    const idTooltip = short.shortened ? fullId : idAnchor ? unnamedHint : undefined;

    return (
        <Box
            component="span"
            aria-label={full}
            data-testid="entity-label"
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, minWidth: 0, maxWidth: '100%', whiteSpace: 'nowrap', ...sx }}
        >
            <Tooltip title={platformAs === 'icon' ? platform : ''}>
                <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'text.secondary', flexShrink: 0 }}>
                    {platformIcon}
                    {platformAs === 'text' || !platformIcon ? platform : null}
                </Box>
            </Tooltip>
            {group && (
                <>
                    {!iconOnly && <Sep />}
                    <Box
                        component="span"
                        sx={{
                            fontSize: '0.6875rem',
                            lineHeight: '17px',
                            px: 0.875,
                            color: 'text.secondary',
                            border: '1px solid',
                            borderColor: 'divider',
                            borderRadius: 9,
                            flexShrink: 0,
                        }}
                    >
                        {groupLabel}
                    </Box>
                </>
            )}
            {aliasText !== '' && (
                <>
                    {!(iconOnly && !group) && <Sep />}
                    <Box component="span" sx={{ fontWeight: 600, color: 'text.primary', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        {aliasText}
                    </Box>
                </>
            )}
            {!(iconOnly && !group && aliasText === '') && <Sep />}
            {idTooltip ? <Tooltip title={idTooltip}>{idNode}</Tooltip> : idNode}
            {!resolved && (
                <Box component="span" sx={{ fontSize: '0.75rem', color: 'warning.main', flexShrink: 0 }}>{deletedLabel}</Box>
            )}
            {copy && (
                <CopyIconButton
                    value={fullId}
                    label={copyLabel}
                    copiedLabel={copiedLabel}
                    onCopied={onCopied}
                    iconSize="0.875rem"
                    sx={{ p: 0.25, flexShrink: 0 }}
                />
            )}
        </Box>
    );
};

export default EntityLabel;
