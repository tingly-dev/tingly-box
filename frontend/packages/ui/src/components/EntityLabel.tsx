import { Box, Tooltip } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { Fragment, type ReactNode } from 'react';
import { fontMono } from '../theme/fonts';
import { DEFAULT_GROUP_LABEL, entityText, groupMark, platformGroupText, shortId, type EntityKind, type EntityParts } from '../utils/entityLabel';
import { CopyIconButton } from './CopyIconButton';
import { fontSizes } from '@tingly/ui/theme/fonts';

export interface EntityLabelProps extends EntityParts {
    /** Drives id shortening; defaults to `group` / `direct` from `group`. */
    kind?: EntityKind;
    /** Brand icon drawn before the platform name. */
    platformIcon?: ReactNode;
    /** `icon` hides the platform text (kept in the tooltip / aria-label) for rows already under a bot header. */
    platformAs?: 'text' | 'icon';
    /** Localised pattern for a group chat's platform segment, with `{{platform}}` ("{{platform}} group", "{{platform}}群"). */
    groupLabel?: string;
    /** False when the referenced entity no longer exists: the stored id is shown with a muted marker. */
    resolved?: boolean;
    deletedLabel?: string;
    /**
     * For tight places (tags, chips): when there is an alias, leave the id out
     * of the line so the alias gets the room. The full label, id included, is
     * the element's tooltip and accessible name.
     */
    compact?: boolean;
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
 * `平台 · alias · id`, with a group chat's marker merged into the platform
 * (`飞书群 · alias · id`) rather than a segment of its own. Presentational only: the caller resolves the
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
    groupLabel = DEFAULT_GROUP_LABEL,
    resolved = true,
    deletedLabel = '(deleted)',
    compact = false,
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
    const showId = !(compact && !idAnchor);
    // A logo alone flows straight into the next segment; a logo with the group mark is a segment like any other.
    const iconOnly = platformAs === 'icon' && Boolean(platformIcon);

    const idNode = (
        <Box
            component="span"
            sx={{
                fontFamily: fontMono,
                fontSize: fontSizes.md,
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

    // Segments in display order. Empty ones are never built, so no separator is
    // ever drawn next to a missing segment; a platform drawn as a logo alone
    // flows straight into the next segment without one.
    const platformText = group ? platformGroupText(platform, groupLabel) : platform;
    const platformNode = (
        <Tooltip title={iconOnly ? platformText : ''}>
            <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'text.secondary', flexShrink: 0 }}>
                {platformIcon}
                {iconOnly ? (group ? groupMark(groupLabel) : null) : platformText}
            </Box>
        </Tooltip>
    );
    const segments: { key: string; node: ReactNode; joinsNext?: boolean }[] = [];
    if (platform !== '' || platformIcon) segments.push({ key: 'platform', node: platformNode, joinsNext: iconOnly && !group });
    if (aliasText !== '') {
        segments.push({
            key: 'alias',
            node: (
                <Box component="span" sx={{ fontWeight: 600, color: 'text.primary', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {aliasText}
                </Box>
            ),
        });
    }
    if (showId) segments.push({ key: 'id', node: idTooltip ? <Tooltip title={idTooltip}>{idNode}</Tooltip> : idNode });

    return (
        <Box
            component="span"
            aria-label={full}
            title={compact ? full : undefined}
            data-testid="entity-label"
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, minWidth: 0, maxWidth: '100%', whiteSpace: 'nowrap', ...sx }}
        >
            {segments.map((seg, i) => (
                <Fragment key={seg.key}>
                    {i > 0 && !segments[i - 1].joinsNext && <Sep />}
                    {seg.node}
                </Fragment>
            ))}
            {!resolved && (
                <Box component="span" sx={{ fontSize: fontSizes.sm, color: 'warning.main', flexShrink: 0 }}>{deletedLabel}</Box>
            )}
            {copy && showId && (
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
