import { useState, type ReactNode } from 'react';
import { Box, ButtonBase, Card, Collapse, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { KeyboardArrowDown } from '@/components/icons';
import TriStateSwitch, { type TriState } from '@tingly/ui/components/TriStateSwitch';
import { radius } from '@tingly/ui/theme/radius';

const FOLD_KEY = (key: string) => `mcp-group-folded-${key}`;

const readFolded = (key?: string) => {
    if (!key) return false;
    try {
        return localStorage.getItem(FOLD_KEY(key)) === 'true';
    } catch {
        return false; // storage unavailable: start open
    }
};

// The one group every MCP tool list uses: a card whose header is the group
// (a connection, a Server Tool, a connection inside a profile) and whose body
// is its tool rows. The header carries "N of M on" and one switch for the whole
// group (mixed when only some are on); rows keep their own switches. With a
// foldKey the group folds to its header and remembers that on this machine.
export default function MCPToolGroup({
    title,
    ariaLabel,
    meta,
    status,
    actions,
    summary,
    on,
    total,
    switchState,
    switchDisabled,
    switchLabel,
    onSwitch,
    foldKey,
    children,
}: {
    title: ReactNode;
    ariaLabel: string;
    // Quiet text right after the title (origin, "Server Tool").
    meta?: ReactNode;
    // Right side of the header, before the actions (connection state).
    status?: ReactNode;
    actions?: ReactNode;
    // Lines under the header that stay visible when folded (target, missing values, errors).
    summary?: ReactNode;
    on: number;
    total: number;
    // Absent: no group switch (a group of one tool has nothing to switch together).
    switchState?: TriState;
    switchDisabled?: boolean;
    switchLabel?: string;
    onSwitch?: (on: boolean) => void;
    foldKey?: string;
    children?: ReactNode;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string, options?: Record<string, unknown>) =>
        t(`mcp.workspace.${key}`, { defaultValue: fallback, ...options });
    const [folded, setFolded] = useState(() => readFolded(foldKey));
    const foldable = !!foldKey && total > 0;
    const fold = () => {
        const next = !folded;
        setFolded(next);
        try {
            localStorage.setItem(FOLD_KEY(foldKey!), String(next));
        } catch {
            /* best-effort */
        }
    };
    const heading = (
        <Typography component="h2" sx={{ fontWeight: 600, fontSize: 14, overflowWrap: 'anywhere', textAlign: 'left' }}>
            {title}
        </Typography>
    );
    return (
        <Card
            component="article"
            variant="outlined"
            aria-label={ariaLabel}
            sx={{ minWidth: 0, borderRadius: radius.card, overflow: 'hidden' }}
        >
            <Stack direction="row" sx={{ alignItems: 'center', gap: 1.25, px: 2, py: 1.25, minHeight: 52, flexWrap: 'wrap' }}>
                {foldable ? (
                    <ButtonBase
                        aria-expanded={!folded}
                        aria-label={folded ? label('expandGroup', 'Show tools') : label('collapseGroup', 'Hide tools')}
                        onClick={fold}
                        sx={{ gap: 1, ml: -0.5, px: 0.5, borderRadius: radius.row, minWidth: 0 }}
                    >
                        <KeyboardArrowDown
                            sx={{
                                fontSize: 18,
                                color: 'text.secondary',
                                transform: folded ? 'rotate(-90deg)' : 'none',
                                transition: 'transform .15s',
                            }}
                        />
                        {heading}
                    </ButtonBase>
                ) : (
                    heading
                )}
                {meta && (
                    <Typography variant="caption" color="text.disabled">
                        {meta}
                    </Typography>
                )}
                <Box sx={{ flex: 1 }} />
                {status}
                {actions}
                {total > 0 && (
                    <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5 }}>
                        <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                            {label('toolsOn', '{{on}} of {{total}} tools on', { on, total })}
                        </Typography>
                        {switchState && onSwitch && (
                            <TriStateSwitch
                                size="small"
                                state={switchState}
                                disabled={switchDisabled}
                                onChange={onSwitch}
                                slotProps={{ input: { 'aria-label': switchLabel || label('groupEnabled', 'All tools on') } }}
                            />
                        )}
                    </Stack>
                )}
            </Stack>
            {summary && <Stack sx={{ px: 2, pb: 1.5, mt: -0.5, gap: 1 }}>{summary}</Stack>}
            <Collapse in={!folded} unmountOnExit>
                {children}
            </Collapse>
        </Card>
    );
}
