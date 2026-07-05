import { Key as IconKey, DeleteOutline as IconDeleteOutline, ContentCopy as IconCopy, AccessTime as IconClock, Person as IconUser, Visibility as IconEye, VisibilityOff as IconEyeOff } from '@/components/icons';
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

export interface SharingKey {
    token_id: string;
    user_id: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at: string;
    created_by?: string;
}

export const maskToken = (token: string): string => {
    if (!token) return '';
    if (token.length <= 16) return `${token.slice(0, 8)}...${token.slice(-4)}`;
    return `${token.slice(0, 12)}...${token.slice(-4)}`;
};

export const formatDate = (dateStr?: string) => {
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
    /** Show the user_id column (default: true) */
    showUserColumn?: boolean;
    /** Show the last_used_at column (default: true) */
    showLastUsedColumn?: boolean;
    /** Label for the user column */
    userColumnLabel?: string;
}

const SharingKeysTable: React.FC<SharingKeysTableProps> = ({
    tokens,
    loading = false,
    visibleTokens,
    onToggleVisibility,
    onCopy,
    onToggleEnabled,
    onDelete,
    showUserColumn = true,
    showLastUsedColumn = true,
    userColumnLabel = 'User',
}) => {
    const colSpan = 2 + (showUserColumn ? 1 : 0) + (showLastUsedColumn ? 1 : 0) + 1; // Name + Token + Status + Created + Actions + optional cols

    return (
        <TableContainer>
            <Table>
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
                        <TableCell>Name</TableCell>
                        {showUserColumn && <TableCell>{userColumnLabel}</TableCell>}
                        <TableCell>Token</TableCell>
                        <TableCell>Status</TableCell>
                        <TableCell>Created</TableCell>
                        {showLastUsedColumn && <TableCell>Last Used</TableCell>}
                        <TableCell align="right">Actions</TableCell>
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
                                <Stack alignItems="center" spacing={1.5} sx={{ py: 4 }}>
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
                                    <Typography variant="body2" color="text.secondary">
                                        No sharing keys yet. Create one to share model access with your team.
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
                                    <Stack direction="row" spacing={1} alignItems="center">
                                        <Box sx={{ color: key.enabled ? 'primary.main' : 'text.disabled', display: 'flex', flexShrink: 0 }}>
                                            <IconKey sx={{ fontSize: 15 }} />
                                        </Box>
                                        <Typography variant="body2" fontWeight={600}>
                                            {key.display_name}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                {showUserColumn && (
                                    <TableCell>
                                        <Tooltip title={key.user_id} placement="top">
                                            <Stack direction="row" spacing={0.5} alignItems="center" sx={{ width: 'fit-content', cursor: 'default' }}>
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
                                        <Tooltip title={visibleTokens[key.token_id] ? 'Hide' : 'Show'}>
                                            <IconButton
                                                size="small"
                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                onClick={() => onToggleVisibility(key.token_id)}
                                            >
                                                {visibleTokens[key.token_id] ? <IconEyeOff sx={{ fontSize: 13 }} /> : <IconEye sx={{ fontSize: 13 }} />}
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title="Copy">
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
                                    <Tooltip title={key.enabled ? 'Active' : 'Disabled'} placement="top">
                                        <Switch
                                            size="small"
                                            checked={key.enabled}
                                            onChange={() => onToggleEnabled(key)}
                                            color="success"
                                        />
                                    </Tooltip>
                                </TableCell>
                                <TableCell>
                                    <Stack direction="row" spacing={0.5} alignItems="center">
                                        <IconClock sx={{ fontSize: 13, opacity: 0.4, flexShrink: 0 }} />
                                        <Typography variant="caption" color="text.secondary">
                                            {formatDate(key.created_at)}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                {showLastUsedColumn && (
                                    <TableCell>
                                        <Typography variant="caption" color="text.secondary">
                                            {formatDate(key.last_used_at)}
                                        </Typography>
                                    </TableCell>
                                )}
                                <TableCell align="right">
                                    <Tooltip title="Delete token">
                                        <IconButton
                                            size="small"
                                            color="error"
                                            onClick={() => onDelete(key)}
                                            sx={{ opacity: 0.75, '&:hover': { opacity: 1 } }}
                                        >
                                            <IconDeleteOutline sx={{ fontSize: 16 }} />
                                        </IconButton>
                                    </Tooltip>
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
