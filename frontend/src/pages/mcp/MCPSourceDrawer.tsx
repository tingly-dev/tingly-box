import { useTranslation } from 'react-i18next';
import { Alert, Divider, Drawer, IconButton, Stack, Typography } from '@mui/material';
import { Close } from '@/components/icons';
import { api } from '@/services/api';
import MCPSourceWorkspace from './MCPSourceWorkspace';
import type { MCPWorkspace } from './useMCPWorkspace';

// One source's workspace in a side sheet, shared by the MCP Tools and Server
// Tools pages (`?source=<id>`).
export default function MCPSourceDrawer({
    ws,
    selectedId,
    onClose,
}: {
    ws: MCPWorkspace;
    selectedId: string | null;
    onClose: () => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const { config, routing, enabled } = ws;
    const selected = selectedId ? (config.sources || []).find((source) => source.id === selectedId) : undefined;
    const closeSheet = onClose;
    return (
        <Drawer
                anchor="right"
                open={!!selectedId}
                onClose={closeSheet}
                slotProps={{
                    paper: {
                        role: 'dialog',
                        'aria-label': selected?.name || selected?.id || label('missingConnection', 'Connection'),
                        sx: { width: { xs: '100%', sm: 680 }, maxWidth: '100%' },
                    },
                }}
            >
                <Stack sx={{ p: 3 }} spacing={2.5}>
                    <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                        <Typography variant="h5" style={{ fontWeight: 600 }}>
                            {selected?.name || selected?.id || label('missingConnection', 'Connection no longer exists')}
                        </Typography>
                        <IconButton aria-label={label('close', 'Close workspace panel')} onClick={closeSheet}>
                            <Close />
                        </IconButton>
                    </Stack>
                    <Divider />
                    {selected ? (
                        <MCPSourceWorkspace
                            key={selected.id}
                            source={selected}
                            route={routing?.sources.find((source) => source.id === selected.id)}
                            enabled={enabled}
                            saveSource={ws.saveSource}
                            onRefresh={async () => {
                                const response = await api.reconnectMCPSource(selected.id!);
                                await ws.refreshRoutes();
                                if (!response.success) throw new Error(response.error || 'Connection check failed');
                            }}
                            onDelete={async () => {
                                await ws.acceptConfig(await api.deleteMCPSource(selected.id!));
                                closeSheet();
                            }}
                        />
                    ) : (
                        <Alert severity="info">{label('missingConnection', 'Connection no longer exists')}</Alert>
                    )}
                </Stack>
            </Drawer>
    );
}
