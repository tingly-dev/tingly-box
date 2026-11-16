import { useState, type ReactNode } from 'react';
import { Box, Button, ButtonBase, Collapse, IconButton, Stack, Switch, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Close, KeyboardArrowDown } from '@/components/icons';
import MCPToolParameters from './MCPToolParameters';
import type { MCPCatalogTool } from './types';

// The one row every tool list uses (MCP Tools, Server Tools, MCP profile): a
// monospace name, a one-line description and a switch. Click the row to see the
// schemas. Test and Remove show on hover or focus. Callers decide what the
// switch means and what is shown when the tool is unavailable.
export default function MCPToolRow({
    name,
    prefix,
    tool,
    checked,
    switchDisabled,
    muted,
    caption,
    note,
    ariaLabel,
    groupRole,
    onToggle,
    onTest,
    testDisabled,
    onRemove,
    removeDisabled,
}: {
    name: string;
    // Source name shown before the tool name where rows from several sources mix.
    prefix?: string;
    // Absent when the tool no longer exists: nothing to expand.
    tool?: MCPCatalogTool;
    checked: boolean;
    switchDisabled?: boolean;
    muted?: boolean;
    // Replaces the description in the row (e.g. why the tool is unavailable).
    caption?: string;
    // Warning line under the row.
    note?: ReactNode;
    ariaLabel: string;
    groupRole?: boolean;
    onToggle: (on: boolean) => void;
    onTest?: () => void;
    testDisabled?: boolean;
    onRemove?: () => void;
    removeDisabled?: boolean;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [open, setOpen] = useState(false);
    const description = caption ?? tool?.description;
    const expandable = !!tool;
    const hoverReveal = { opacity: { xs: 1, sm: 0 }, '&:focus-visible': { opacity: 1 } };
    return (
        <Box
            component={groupRole ? 'div' : 'article'}
            role={groupRole ? 'group' : undefined}
            aria-label={ariaLabel}
            sx={{
                borderTop: '1px solid',
                borderColor: 'divider',
                '&:hover .row-reveal, &:focus-within .row-reveal': { opacity: 1 },
                '&:hover': { bgcolor: 'action.hover' },
            }}
        >
            <Stack direction="row" sx={{ alignItems: 'center', gap: 1, px: 2, minHeight: 40 }}>
                <ButtonBase
                    aria-expanded={expandable ? open : undefined}
                    disabled={!expandable}
                    onClick={() => setOpen((v) => !v)}
                    sx={{ flex: 1, minWidth: 0, gap: 1.5, py: 1, justifyContent: 'flex-start', textAlign: 'left' }}
                >
                    <KeyboardArrowDown
                        sx={{
                            fontSize: 16,
                            color: 'text.disabled',
                            visibility: expandable ? 'visible' : 'hidden',
                            transform: open ? 'none' : 'rotate(-90deg)',
                            transition: 'transform .15s',
                        }}
                    />
                    <Typography
                        sx={{
                            fontFamily: 'monospace',
                            fontSize: 12,
                            fontWeight: 500,
                            width: { xs: 'auto', sm: 240 },
                            flexShrink: 0,
                            overflowWrap: 'anywhere',
                            color: muted ? 'text.disabled' : 'text.primary',
                        }}
                    >
                        {prefix && (
                            <Box component="span" sx={{ color: 'text.disabled', fontWeight: 400 }}>
                                {prefix} /{' '}
                            </Box>
                        )}
                        {name}
                    </Typography>
                    {description && (
                        <Typography
                            noWrap
                            sx={{
                                display: { xs: 'none', sm: 'block' },
                                fontSize: 12,
                                minWidth: 0,
                                color: muted ? 'text.disabled' : 'text.secondary',
                            }}
                        >
                            {description}
                        </Typography>
                    )}
                </ButtonBase>
                {onTest && (
                    <Button size="small" className="row-reveal" sx={hoverReveal} disabled={testDisabled} onClick={onTest}>
                        {label('testTool', 'Test tool')}
                    </Button>
                )}
                {onRemove && (
                    <IconButton
                        size="small"
                        className="row-reveal"
                        sx={hoverReveal}
                        disabled={removeDisabled}
                        aria-label={label('removeFromProfile', 'Remove from profile')}
                        onClick={onRemove}
                    >
                        <Close fontSize="small" />
                    </IconButton>
                )}
                <Switch
                    size="small"
                    disabled={switchDisabled}
                    checked={checked}
                    onChange={(e) => onToggle(e.target.checked)}
                    slotProps={{ input: { 'aria-label': tool ? label('toolEnabled', 'On') : name } }}
                />
            </Stack>
            {note && (
                <Typography variant="caption" color="warning.main" sx={{ display: 'block', px: 2, pb: 1, pl: 6.5 }}>
                    {note}
                </Typography>
            )}
            {tool && (
                <Collapse in={open} unmountOnExit>
                    <Box sx={{ px: 2, pl: 6.5, pb: 1.5 }}>
                        {tool.description && (
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                {tool.description}
                            </Typography>
                        )}
                        <MCPToolParameters tool={tool} />
                    </Box>
                </Collapse>
            )}
        </Box>
    );
}
