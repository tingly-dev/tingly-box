import { useState } from 'react';
import { Box, ButtonBase, Button, Collapse, Stack, Switch, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { KeyboardArrowDown } from '@/components/icons';
import MCPToolParameters from './MCPToolParameters';
import { isAdvisorTool, isServerToolSource, needsSearchKey, isToolRestricted, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPSourceConfig } from './types';

// One tool as a compact row with its global switch. Off here means off in
// every profile. Click the row to see its schemas; Test shows on hover/focus.
export default function MCPToolCard({
    tool,
    source,
    enabled,
    busy,
    onChange,
    onTest,
    inWorkspace = false,
}: {
    tool: MCPCatalogTool;
    source?: MCPSourceConfig;
    enabled: boolean;
    busy: boolean;
    onChange: (patch: MCPToolPatch) => void;
    onTest?: () => void;
    inWorkspace?: boolean;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [open, setOpen] = useState(false);
    const advisor = isAdvisorTool(tool, source);
    const serverTool = isServerToolSource(source);
    const missingSearchKey = tool.name === 'mcp_web_search' && needsSearchKey(source);
    const restricted = isToolRestricted(tool, source);
    const muted = !tool.enabled || restricted;
    const note = missingSearchKey
        ? label('missing_searchKey', 'Search needs a Serper API key; Fetch does not.')
        : restricted
          ? source?.enabled === false
              ? label('connectionOff', 'This connection is off. Enable it to discover and use its tools.')
              : label('restrictedTool', 'Excluded by the connection allow list. Adjust it in connection settings.')
          : '';
    return (
        <Box
            component={inWorkspace ? 'div' : 'article'}
            role={inWorkspace ? 'group' : undefined}
            aria-label={inWorkspace ? tool.name : `${source?.name || tool.source_id} / ${tool.name}`}
            sx={{
                borderTop: '1px solid',
                borderColor: 'divider',
                '&:hover .tool-test, &:focus-within .tool-test': { opacity: 1 },
                '&:hover': { bgcolor: 'action.hover' },
            }}
        >
            <Stack direction="row" sx={{ alignItems: 'center', gap: 1, px: 2, minHeight: 40 }}>
                <ButtonBase
                    aria-expanded={open}
                    onClick={() => setOpen((v) => !v)}
                    sx={{ flex: 1, minWidth: 0, gap: 1.5, py: 1, justifyContent: 'flex-start', textAlign: 'left' }}
                >
                    <KeyboardArrowDown
                        sx={{
                            fontSize: 16,
                            color: 'text.disabled',
                            transform: open ? 'none' : 'rotate(-90deg)',
                            transition: 'transform .15s',
                        }}
                    />
                    <Typography
                        sx={{
                            fontFamily: 'monospace',
                            fontSize: 12,
                            fontWeight: 500,
                            width: { xs: 'auto', sm: 200 },
                            flexShrink: 0,
                            overflowWrap: 'anywhere',
                            color: muted ? 'text.disabled' : 'text.primary',
                        }}
                    >
                        {tool.name}
                    </Typography>
                    {tool.description && (
                        <Typography
                            noWrap
                            sx={{
                                display: { xs: 'none', sm: 'block' },
                                fontSize: 12,
                                minWidth: 0,
                                color: muted ? 'text.disabled' : 'text.secondary',
                            }}
                        >
                            {tool.description}
                        </Typography>
                    )}
                </ButtonBase>
                {onTest && (
                    <Button
                        size="small"
                        className="tool-test"
                        sx={{ opacity: { xs: 1, sm: 0 }, '&:focus-visible': { opacity: 1 } }}
                        disabled={busy || (!enabled && !serverTool) || !tool.enabled || restricted || advisor || missingSearchKey}
                        onClick={onTest}
                    >
                        {label('testTool', 'Test tool')}
                    </Button>
                )}
                <Switch
                    size="small"
                    disabled={busy || restricted}
                    checked={tool.enabled}
                    onChange={(e) => onChange({ enabled: e.target.checked })}
                    slotProps={{ input: { 'aria-label': label('toolEnabled', 'On') } }}
                />
            </Stack>
            {note && (
                <Typography variant="caption" color="warning.main" sx={{ display: 'block', px: 2, pb: 1, pl: 6.5 }}>
                    {note}
                </Typography>
            )}
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
        </Box>
    );
}
