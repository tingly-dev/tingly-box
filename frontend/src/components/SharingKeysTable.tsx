import { Key as IconKey, DeleteOutline as IconDeleteOutline, ContentCopy as IconCopy, AccessTime as IconClock, Person as IconUser, Visibility as IconEye, VisibilityOff as IconEyeOff, CompareArrows as IconMove } from '@/components/icons';
import {
    Box,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
    IconButton,
    Tooltip,
    Switch,
    CircularProgress,
} from '@mui/material';
import { useTranslation } from 'react-i18next';

export interface SharingKey {
    token_id: string;
    user_id: string;
    team_id?: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at: string;
    created_by?: string;
}

const maskToken = (token: string): string => {
    if (!token) return '';
    if (token.length <= 16) return `${token.slice(0, 8)}...${token.slice(-4)}`;
    return `${token.slice(0, 12)}...${token.slice(-4)}`;
};

const formatDate = (dateStr?: string) => {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
};

interface SharingKeysTableProps {
    tokens: SharingKey[];
    loading?: boolean;
    visibleTokens: Record<string, boolean>;
    onToggleVisibility: (tokenId: string) => void;
    onCopy: (tokenId: string) => void;
    onToggleEnabled: (token: SharingKey) => void;
    onDelete: (token: SharingKey) => void;
    onMove?: (token: SharingKey) => void;
    /** Show the user_id column (default: true) */
    showUserColumn?: boolean;
    /** Show the last_used_at column (default: true) */
    showLastUsedColumn?: boolean;
}

const SharingKeysTable: React.FC<SharingKeysTableProps> = ({
    tokens,
    loading = false,
    visibleTokens,
    onToggleVisibility,
    onCopy,
    onToggleEnabled,
    onDelete,
    onMove,
    showUserColumn = true,
    showLastUsedColumn = true,
}) => {
    const { t } = useTranslation();
    const colSpan = 5 + (showUserColumn ? 1 : 0) + (showLastUsedColumn ? 1 : 0);

    return (
        <TableContainer>
            {/* Fixed layout + explicit widths so several tables stacked on one
                page (one per Team) line their columns up; minWidth keeps the
                columns readable and lets narrow screens scroll the table
                instead of the page. */}
            <Table sx={{ tableLayout: 'fixed', minWidth: 760 }}>
                <colgroup>
                    <col style={{ width: '20%' }} />
                    {showUserColumn && <col style={{ width: '10%' }} />}
                    <col style={{ width: '22%' }} />
                    <col style={{ width: 88 }} />
                    <col style={{ width: '18%' }} />
                    {showLastUsedColumn && <col style={{ width: '18%' }} />}
                    <col style={{ width: 96 }} />
                </colgroup>
                <TableHead>
                    <TableRow
                        sx={{
                            bgcolor: 'action.hover',
                            '& th': {
                                fontWeight: 700,
                                fontSize: '0.75rem',
                                textTransform: 'uppercase',
                                letterSpacing: '0.05em',
                                color: 'text.secondary',
                                borderBottom: '2px solid',
                                borderColor: 'divider',
                                py: 1.5,
                            },
                        }}
                    >
                        <TableCell>{t('sharingKeys.table.name')}</TableCell>
                        {showUserColumn && <TableCell>{t('sharingKeys.table.user')}</TableCell>}
                        <TableCell>{t('sharingKeys.table.token')}</TableCell>
                        <TableCell>{t('sharingKeys.table.status')}</TableCell>
                        <TableCell>{t('sharingKeys.table.created')}</TableCell>
                        {showLastUsedColumn && <TableCell>{t('sharingKeys.table.lastUsed')}</TableCell>}
                        <TableCell align="right">{t('sharingKeys.table.actions')}</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {loading ? (
                        <TableRow>
                            <TableCell colSpan={colSpan} align="center" sx={{ border: 0 }}>
                                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                                    <CircularProgress />
                                </Box>
                            </TableCell>
                        </TableRow>
                    ) : tokens.length === 0 ? (
                        <TableRow>
                            <TableCell colSpan={colSpan} align="center" sx={{ border: 0 }}>
                                <Stack
                                    spacing={1.5}
                                    sx={{
                                        alignItems: "center",
                                        py: 4
                                    }}>
                                    <Box
                                        sx={{
                                            width: 40,
                                            height: 40,
                                            borderRadius: '50%',
                                            bgcolor: 'action.hover',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            color: 'text.disabled',
                                        }}
                                    >
                                        <IconKey sx={{ fontSize: 20 }} />
                                    </Box>
                                    <Typography variant="body2" sx={{
                                        color: "text.secondary"
                                    }}>
                                        {t('sharingKeys.table.empty')}
                                    </Typography>
                                </Stack>
                            </TableCell>
                        </TableRow>
                    ) : (
                        tokens.map((key) => (
                            <TableRow
                                key={key.token_id}
                                sx={{
                                    '&:last-child td': { border: 0 },
                                    opacity: !key.enabled ? 0.55 : 1,
                                    '&:hover': { bgcolor: 'action.hover' },
                                    '& td': { py: 1.5 },
                                }}
                            >
                                <TableCell>
                                    <Stack direction="row" spacing={1} sx={{
                                        alignItems: "center"
                                    }}>
                                        <Box sx={{ color: key.enabled ? 'primary.main' : 'text.disabled', display: 'flex', flexShrink: 0 }}>
                                            <IconKey sx={{ fontSize: 15 }} />
                                        </Box>
                                        <Typography variant="body2" noWrap title={key.display_name} sx={{
                                            fontWeight: 600,
                                            minWidth: 0,
                                        }}>
                                            {key.display_name}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                {showUserColumn && (
                                    <TableCell>
                                        <Tooltip title={key.user_id} placement="top">
                                            <Stack
                                                direction="row"
                                                spacing={0.5}
                                                sx={{
                                                    alignItems: "center",
                                                    width: 'fit-content',
                                                    cursor: 'default'
                                                }}>
                                                <IconUser sx={{ fontSize: 13, opacity: 0.4, flexShrink: 0 }} />
                                                <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                                                    {key.user_id.slice(0, 8)}…
                                                </Typography>
                                            </Stack>
                                        </Tooltip>
                                    </TableCell>
                                )}
                                <TableCell sx={{ maxWidth: 240 }}>
                                    <Box
                                        sx={{
                                            px: 1, py: 0.5,
                                            bgcolor: 'action.selected',
                                            borderRadius: 1,
                                            border: '1px solid',
                                            borderColor: 'divider',
                                            display: 'flex',
                                            alignItems: 'center',
                                            gap: 0.5,
                                        }}
                                    >
                                        <Typography
                                            variant="caption"
                                            sx={{
                                                fontFamily: 'monospace',
                                                flex: 1,
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                color: 'text.secondary',
                                            }}
                                        >
                                            {visibleTokens[key.token_id] ? key.token_id : maskToken(key.token_id)}
                                        </Typography>
                                        <Tooltip title={visibleTokens[key.token_id] ? t('sharingKeys.table.hide') : t('sharingKeys.table.show')}>
                                            <IconButton
                                                size="small"
                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                onClick={() => onToggleVisibility(key.token_id)}
                                            >
                                                {visibleTokens[key.token_id] ? <IconEyeOff sx={{ fontSize: 13 }} /> : <IconEye sx={{ fontSize: 13 }} />}
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title={t('sharingKeys.table.copy')}>
                                            <IconButton
                                                size="small"
                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                onClick={() => onCopy(key.token_id)}
                                            >
                                                <IconCopy sx={{ fontSize: 13 }} />
                                            </IconButton>
                                        </Tooltip>
                                    </Box>
                                </TableCell>
                                <TableCell>
                                    <Tooltip title={key.enabled ? t('sharingKeys.table.active') : t('sharingKeys.table.inactive')} placement="top">
                                        <Switch
                                            size="small"
                                            checked={key.enabled}
                                            onChange={() => onToggleEnabled(key)}
                                            color="success"
                                        />
                                    </Tooltip>
                                </TableCell>
                                <TableCell>
                                    <Stack direction="row" spacing={0.5} sx={{
                                        alignItems: "center"
                                    }}>
                                        <IconClock sx={{ fontSize: 13, opacity: 0.4, flexShrink: 0 }} />
                                        <Typography variant="caption" sx={{
                                            color: "text.secondary"
                                        }}>
                                            {formatDate(key.created_at)}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                {showLastUsedColumn && (
                                    <TableCell>
                                        <Typography variant="caption" sx={{
                                            color: "text.secondary"
                                        }}>
                                            {formatDate(key.last_used_at)}
                                        </Typography>
                                    </TableCell>
                                )}
                                <TableCell align="right">
                                    <Stack direction="row" spacing={0.5} sx={{justifyContent: 'flex-end'}}>
                                        {onMove && (
                                            <Tooltip title={t('sharingKeys.table.moveTooltip')}>
                                                <IconButton size="small" onClick={() => onMove(key)}>
                                                    <IconMove sx={{ fontSize: 16 }} />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <Tooltip title={t('sharingKeys.table.deleteTooltip')}>
                                            <IconButton
                                                size="small"
                                                color="error"
                                                onClick={() => onDelete(key)}
                                                sx={{ opacity: 0.75, '&:hover': { opacity: 1 } }}
                                            >
                                                <IconDeleteOutline sx={{ fontSize: 16 }} />
                                            </IconButton>
                                        </Tooltip>
                                    </Stack>
                                </TableCell>
                            </TableRow>
                        ))
                    )}
                </TableBody>
            </Table>
        </TableContainer>
    );
};

export default SharingKeysTable;
