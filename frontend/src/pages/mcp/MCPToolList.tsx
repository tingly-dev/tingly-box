import { useState } from 'react';
import { Box, Button, InputAdornment, Menu, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { KeyboardArrowDown, Search } from '@/components/icons';
import MCPToolCard from './MCPToolCard';
import { isToolRestricted, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPSourceConfig } from './types';

const FOLDED = 5;
const FILTERABLE = 8;

// A connection's tools as one list: how many are on, a filter and all on/off
// for long lists, and only the first few shown until asked.
export default function MCPToolList({
    tools,
    source,
    enabled,
    busy,
    onChange,
    onBulk,
    onTest,
}: {
    tools: MCPCatalogTool[];
    source: MCPSourceConfig;
    enabled: boolean;
    busy: boolean;
    onChange: (tool: MCPCatalogTool, patch: MCPToolPatch) => void;
    onBulk: (tools: MCPCatalogTool[], enabled: boolean) => void;
    onTest: (tool: MCPCatalogTool) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string, options?: Record<string, unknown>) =>
        t(`mcp.workspace.${key}`, { defaultValue: fallback, ...options });
    const [filter, setFilter] = useState('');
    const [all, setAll] = useState(false);
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    const query = filter.trim().toLowerCase();
    const matched = query
        ? tools.filter((tool) => `${tool.name} ${tool.description || ''}`.toLowerCase().includes(query))
        : tools;
    const shown = all || query ? matched : matched.slice(0, FOLDED);
    const on = tools.filter((tool) => tool.enabled && !isToolRestricted(tool, source)).length;
    const bulk = (value: boolean) => {
        setMenu(null);
        onBulk(matched, value);
    };
    return (
        <Box>
            <Stack
                direction="row"
                sx={{
                    alignItems: 'center',
                    gap: 1,
                    px: 2,
                    py: 1,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    bgcolor: 'action.hover',
                }}
            >
                <Typography variant="caption" sx={{ fontWeight: 600, flex: 1 }}>
                    {label('toolsOn', '{{on}} of {{total}} tools on', { on, total: tools.length })}
                </Typography>
                {tools.length > FILTERABLE && (
                    <TextField
                        size="small"
                        value={filter}
                        placeholder={label('filterTools', 'Filter tools…')}
                        onChange={(e) => setFilter(e.target.value)}
                        slotProps={{
                            htmlInput: { 'aria-label': label('filterTools', 'Filter tools…') },
                            input: {
                                startAdornment: (
                                    <InputAdornment position="start">
                                        <Search sx={{ fontSize: 16 }} />
                                    </InputAdornment>
                                ),
                                sx: { fontSize: 12, height: 28, bgcolor: 'background.paper' },
                            },
                        }}
                    />
                )}
                {tools.length > 1 && (
                    <>
                        <Button
                            size="small"
                            color="inherit"
                            disabled={busy}
                            endIcon={<KeyboardArrowDown sx={{ fontSize: 16 }} />}
                            onClick={(e) => setMenu(e.currentTarget)}
                            sx={{ color: 'text.secondary' }}
                        >
                            {query ? label('bulkMatching', 'Matching') : label('bulkAll', 'All')}
                        </Button>
                        <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)}>
                            <MenuItem onClick={() => bulk(true)}>{label('turnAllOn', 'Turn on')}</MenuItem>
                            <MenuItem onClick={() => bulk(false)}>{label('turnAllOff', 'Turn off')}</MenuItem>
                        </Menu>
                    </>
                )}
            </Stack>
            {shown.map((tool) => (
                <MCPToolCard
                    key={tool.normalized_name}
                    tool={tool}
                    source={source}
                    enabled={enabled}
                    busy={busy}
                    onChange={(patch) => onChange(tool, patch)}
                    onTest={() => onTest(tool)}
                />
            ))}
            {query && matched.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ px: 2, py: 1.5, borderTop: '1px solid', borderColor: 'divider' }}>
                    {label('noMatchingTools', 'No tools match.')}
                </Typography>
            )}
            {!query && matched.length > FOLDED && (
                <Box sx={{ borderTop: '1px solid', borderColor: 'divider', px: 1, py: 0.5 }}>
                    <Button size="small" onClick={() => setAll((v) => !v)}>
                        {all
                            ? label('showFewerTools', 'Show fewer')
                            : label('showAllTools', 'Show all {{count}} tools', { count: matched.length })}
                    </Button>
                </Box>
            )}
        </Box>
    );
}
