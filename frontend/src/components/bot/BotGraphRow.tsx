import type { BotSettings } from '@/types/bot';
import type { Provider } from '@/types/provider';
import {
    ChevronRight as ChevronRightIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import {
    Box,
    Collapse,
    IconButton,
    Stack,
    Switch,
    TableCell,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import { useCallback } from 'react';
import RemoteControlGraph from './RemoteControlGraph.tsx';

const GraphRowContainer = styled(TableRow)({
    '&:hover': { backgroundColor: 'transparent' },
});

const GraphCell = styled(TableCell)(({ theme }) => ({
    backgroundColor: theme.palette.grey[50],
    borderBottom: '1px solid',
    borderBottomColor: theme.palette.divider,
    padding: theme.spacing(2),
    '&:first-child': { paddingLeft: theme.spacing(2) },
    '&:last-child': { paddingRight: theme.spacing(2) },
}));

interface BotGraphRowProps {
    bot: BotSettings;
    providers: Provider[];
    isExpanded: boolean;
    onToggleExpand: () => void;
    onCWDChange: (cwd: string) => void;
    onModelClick?: () => void;
    onBotToggle?: (uuid: string, enabled: boolean) => void;
    onEdit?: () => void;
    onDelete?: () => void;
    readOnly?: boolean;
    isToggling?: boolean;
}

const BotGraphRow: React.FC<BotGraphRowProps> = ({
    bot,
    providers,
    isExpanded,
    onToggleExpand,
    onCWDChange,
    onModelClick,
    onBotToggle,
    onEdit,
    onDelete,
    readOnly = false,
    isToggling = false,
}) => {
    const isBotEnabled = bot.enabled ?? true;

    const handleBotToggle = useCallback((enabled: boolean) => {
        onBotToggle?.(bot.uuid || '', enabled);
    }, [onBotToggle, bot.uuid]);

    const handleActionClick = useCallback((callback: () => void) => {
        return (e: React.MouseEvent) => {
            e.stopPropagation();
            callback();
        };
    }, []);

    return (
        <>
            <TableRow sx={{ cursor: 'pointer', '&:hover': { backgroundColor: 'action.hover' } }} onClick={onToggleExpand}>
                <TableCell sx={{ width: 60 }}>
                    <IconButton size="small">{isExpanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}</IconButton>
                </TableCell>
                <TableCell sx={{ minWidth: 120 }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>{bot.name || bot.platform}</Typography>
                </TableCell>
                <TableCell sx={{ minWidth: 140 }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>{bot.platform}</Typography>
                </TableCell>
                <TableCell sx={{ minWidth: 80 }} align="center">
                    {bot.proxy_url ? (
                        <Typography variant="caption" color="text.secondary">Configured</Typography>
                    ) : (
                        <Typography variant="body2" color="text.secondary">-</Typography>
                    )}
                </TableCell>
                <TableCell sx={{ minWidth: 120 }}>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{bot.chat_id || '-'}</Typography>
                </TableCell>
                <TableCell sx={{ minWidth: 150 }}>
                    <Typography variant="body2" sx={{
                        fontWeight: 500,
                        color: isBotEnabled ? 'success.main' : 'text.secondary',
                    }}>
                        Claude Code
                    </Typography>
                </TableCell>
                <TableCell sx={{ minWidth: 100 }}>
                    <Stack direction="row" spacing={0.5} alignItems="center">
                        <Tooltip title={isBotEnabled ? 'Disable Bot' : 'Enable Bot'}>
                            <Switch
                                checked={isBotEnabled}
                                onChange={(e) => { e.stopPropagation(); handleBotToggle(e.target.checked); }}
                                size="small"
                                color="success"
                                onClick={(e) => e.stopPropagation()}
                                disabled={isToggling}
                            />
                        </Tooltip>
                        {onEdit && (
                            <Tooltip title="Edit">
                                <IconButton size="small" color="primary" onClick={handleActionClick(onEdit)} disabled={isToggling}>
                                    <EditIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        )}
                        {onDelete && (
                            <Tooltip title="Delete">
                                <IconButton size="small" color="error" onClick={handleActionClick(onDelete)} disabled={isToggling}>
                                    <DeleteIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        )}
                    </Stack>
                </TableCell>
            </TableRow>

            <GraphRowContainer>
                <GraphCell colSpan={7}>
                    <Collapse in={true} timeout="auto" unmountOnExit>
                        <Box onClick={(e) => e.stopPropagation()}>
                            <RemoteControlGraph
                                imbot={bot}
                                providers={providers}
                                currentCWD={bot.default_cwd || ''}
                                isBotEnabled={isBotEnabled}
                                readOnly={readOnly}
                                onCWDChange={onCWDChange}
                                onModelClick={onModelClick}
                                onBotClick={onEdit}
                            />
                        </Box>
                    </Collapse>
                </GraphCell>
            </GraphRowContainer>
        </>
    );
};

export default BotGraphRow;
