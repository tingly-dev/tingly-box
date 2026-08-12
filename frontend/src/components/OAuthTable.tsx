import {ApiStyleBadge} from "@/components/ApiStyleBadge.tsx";
import ModelListDialog from "@/components/ModelListDialog";
import type {ExportFormat} from "@/components/rule-card/utils";
import {
    exportProviderAsBase64ToClipboard,
    exportProviderAsJsonlToClipboard,
} from "@/components/rule-card/utils";
import {ProviderQuotaDetailRow} from "@/components/credential/ProviderQuotaDetailRow";
import {
    ContentCopy,
    DataUsage,
    Delete,
    Edit,
    ListAlt,
    MoreVert,
    Refresh as RefreshIcon,
    Route,
    Schedule,
    VpnKey,
} from '@/components/icons';
import {
    Box,
    Button,
    CircularProgress,
    Divider,
    IconButton,
    Menu,
    MenuItem,
    Modal,
    Paper,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from "@mui/material";
import type {ProviderQuota} from "@/types/quota";
import React, {useCallback, useState} from "react";
import type {Provider} from "../types/provider";

interface OAuthTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onReauthorize?: (providerUuid: string) => void;
    onRefreshToken?: (providerUuid: string) => Promise<void>;
    onNotification?: (message: string, severity: "success" | "error") => void;
    providerQuotas?: { [uuid: string]: ProviderQuota };
    refreshingQuotas?: Set<string>;
    onQuotaRefresh?: (providerUuid: string) => void;
}

interface DeleteModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

interface RefreshModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

interface ModelListDialogState {
    open: boolean;
    provider: Provider | null;
}

// Column widths, in one place — tweak here rather than hunting through the
// TableHead/TableBody JSX. `align`/`sx` are merged onto the header
// TableCell's default styling; body cells still render their own content.
const COLUMNS: { label: string; width: number; align?: "center"; sx?: object }[] = [
    {label: "Status", width: 72},
    {label: "Name", width: 140},
    {label: "API Style", width: 88, align: "center", sx: {px: 1, whiteSpace: "nowrap"}},
    {label: "Provider", width: 150},
    {label: "Expires At", width: 140},
    {label: "Proxy", width: 60},
    {label: "Actions", width: 190},
];
const TABLE_MIN_WIDTH = COLUMNS.reduce((sum, c) => sum + c.width, 0);

const OAuthTable = ({
                        providers,
                        onEdit,
                        onToggle,
                        onDelete,
                        onReauthorize,
                        onRefreshToken,
                        onNotification,
                        providerQuotas,
                        refreshingQuotas,
                        onQuotaRefresh,
                    }: OAuthTableProps) => {
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerUuid: "",
        providerName: "",
    });

    const [refreshModal, setRefreshModal] = useState<RefreshModalState>({
        open: false,
        providerUuid: "",
        providerName: "",
    });

    const [refreshing, setRefreshing] = useState<string | null>(null);

    const [modelListDialog, setModelListDialog] = useState<ModelListDialogState>({
        open: false,
        provider: null,
    });
    const [moreMenu, setMoreMenu] = useState<{
        anchorEl: HTMLElement | null;
        providerUuid: string;
    }>({
        anchorEl: null,
        providerUuid: "",
    });

    const handleMoreOpen = (
        e: React.MouseEvent<HTMLElement>,
        providerUuid: string,
    ) => {
        e.stopPropagation();
        setMoreMenu({anchorEl: e.currentTarget, providerUuid});
    };
    const handleMoreClose = () =>
        setMoreMenu({anchorEl: null, providerUuid: ""});

    const handleDeleteClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        setDeleteModal({
            open: true,
            providerUuid,
            providerName: provider?.name || "Unknown Provider",
        });
    };

    const handleCloseDeleteModal = () => {
        setDeleteModal({open: false, providerUuid: "", providerName: ""});
    };

    const handleConfirmDelete = () => {
        if (onDelete && deleteModal.providerUuid) {
            onDelete(deleteModal.providerUuid);
        }
        handleCloseDeleteModal();
    };

    const handleRefreshClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        setRefreshModal({
            open: true,
            providerUuid,
            providerName: provider?.name || "Unknown Provider",
        });
    };

    const handleCloseRefreshModal = () => {
        setRefreshModal({open: false, providerUuid: "", providerName: ""});
    };

    const handleConfirmRefresh = async () => {
        if (!onRefreshToken || !refreshModal.providerUuid) return;

        setRefreshing(refreshModal.providerUuid);
        try {
            await onRefreshToken(refreshModal.providerUuid);
        } finally {
            setRefreshing(null);
        }
        handleCloseRefreshModal();
    };

    const handleModelListClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        if (provider) {
            setModelListDialog({open: true, provider});
        }
    };

    const handleCloseModelListDialog = () => {
        setModelListDialog({open: false, provider: null});
    };

    const handleCopyProviderBase64 = useCallback(
        async (provider: Provider) => {
            await exportProviderAsBase64ToClipboard(provider, (message, severity) => {
                onNotification?.(message, severity);
            });
        },
        [onNotification],
    );

    const handleCopyProviderJsonl = useCallback(
        async (provider: Provider) => {
            await exportProviderAsJsonlToClipboard(provider, (message, severity) => {
                onNotification?.(message, severity);
            });
        },
        [onNotification],
    );

    const formatExpiresAt = (expiresAt?: string) => {
        if (!expiresAt) return "Never";
        const date = new Date(expiresAt);
        const now = new Date();
        const isExpired = date < now;

        // Format as relative time
        const diffMs = date.getTime() - now.getTime();
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMs / 3600000);
        const diffDays = Math.floor(diffMs / 86400000);

        if (isExpired) {
            return "Expired";
        } else if (diffMins < 60) {
            return `in ${diffMins} min`;
        } else if (diffHours < 24) {
            return `in ${diffHours}h`;
        } else if (diffDays < 7) {
            return `in ${diffDays} days`;
        } else {
            // For longer periods, show date
            return date.toLocaleDateString();
        }
    };

    const getExpirationColor = (expiresAt?: string) => {
        if (!expiresAt) return "default";
        const date = new Date(expiresAt);
        const now = new Date();
        const diffMs = date.getTime() - now.getTime();
        const diffHours = diffMs / 3600000;

        if (date < now) return "error";
        if (diffHours < 1) return "error";
        if (diffHours < 24) return "warning";
        return "success";
    };

    return (
        <TableContainer
            component={Paper}
            elevation={0}
            sx={{
                border: 1,
                borderColor: "divider",
                borderRadius: 2,
                boxShadow: "none",
                overflowX: "auto",
            }}
        >
            {/* Fixed column widths (see COLUMNS above); the table itself
                scrolls horizontally below minWidth instead of columns resizing. */}
            <Table sx={{tableLayout: "fixed", width: '100%', minWidth: TABLE_MIN_WIDTH}}>
                <TableHead>
                    <TableRow sx={{bgcolor: "action.hover"}}>
                        {COLUMNS.map((col) => (
                            <TableCell
                                key={col.label}
                                align={col.align}
                                sx={{fontWeight: 600, width: col.width, py: 1.25, ...col.sx}}
                            >
                                {col.label}
                            </TableCell>
                        ))}
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => {
                        const expiresAt = provider.oauth_detail?.expires_at;

                        return (
                            <React.Fragment key={provider.uuid}>
                                {/* Main provider row */}
                                <TableRow
                                    hover
                                    sx={{
                                        "& > .MuiTableCell-root": {
                                            py: 1.25,
                                        },
                                    }}
                                >
                                    {/* Status */}
                                    <TableCell>
                                        <Stack direction="row" spacing={1} sx={{
                                            alignItems: "center"
                                        }}>
                                            <Switch
                                                checked={provider.enabled}
                                                onChange={() => onToggle?.(provider.uuid)}
                                                size="small"
                                                color="success"
                                            />
                                        </Stack>
                                    </TableCell>
                                    {/* Name */}
                                    <TableCell>
                                        <Stack direction="row" spacing={1} sx={{
                                            alignItems: "center"
                                        }}>
                                            <Tooltip
                                                arrow
                                                placement="top"
                                                title={(
                                                    <Box>
                                                        <Typography variant="caption" sx={{display: 'block', color: 'inherit', fontWeight: 600}}>
                                                            {provider.name}
                                                        </Typography>
                                                        <Typography variant="caption" sx={{display: 'block', color: 'inherit', fontFamily: 'monospace', opacity: 0.8}}>
                                                            UUID: {provider.uuid}
                                                        </Typography>
                                                    </Box>
                                                )}
                                            >
                                                <Typography
                                                    variant="body2"
                                                    sx={{fontWeight: 500, maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap"}}
                                                >
                                                    {provider.name}
                                                </Typography>
                                            </Tooltip>
                                        </Stack>
                                    </TableCell>
                                    {/* API Style */}
                                    <TableCell align="center" sx={{px: 1}}>
                                        <Box sx={{display: 'flex', justifyContent: 'center'}}>
                                            <ApiStyleBadge
                                                minimal
                                                minimalSize="medium"
                                                apiStyle={provider.api_style}
                                            />
                                        </Box>
                                    </TableCell>
                                    {/* Provider Type */}
                                    <TableCell>
                                        <Typography
                                            variant="body2"
                                            sx={{textTransform: "capitalize"}}
                                        >
                                            {provider.oauth_detail?.issuer || "N/A"}
                                        </Typography>
                                    </TableCell>
                                    {/* Expires At */}
                                    <TableCell>
                                        <Stack direction="row" spacing={1} sx={{
                                            alignItems: "center"
                                        }}>
                                            <Schedule
                                                fontSize="small"
                                                color={getExpirationColor(expiresAt) as any}
                                            />
                                            <Typography
                                                variant="body2"
                                                color={(getExpirationColor(expiresAt) + ".main") as any}
                                            >
                                                {formatExpiresAt(expiresAt)}
                                            </Typography>
                                        </Stack>
                                    </TableCell>
                                    {/* Proxy */}
                                    <TableCell align="center">
                                        {provider.proxy_url ? (
                                            <Tooltip title={provider.proxy_url} arrow>
                                                <Route
                                                    fontSize="small"
                                                    sx={{color: "text.secondary"}}
                                                />
                                            </Tooltip>
                                        ) : (
                                            <Typography variant="body2" sx={{
                                                color: "text.secondary"
                                            }}>
                                                -
                                            </Typography>
                                        )}
                                    </TableCell>
                                    {/* Actions */}
                                    <TableCell sx={{whiteSpace: "nowrap"}}>
                                        <Box
                                            sx={{
                                                display: "flex",
                                                alignItems: "center",
                                                gap: 0.5,
                                                border: 1,
                                                borderColor: "divider",
                                                borderRadius: 1.5,
                                                p: 0.5,
                                                width: "fit-content",
                                            }}
                                        >
                                            {/* Edit — primary action */}
                                            {onEdit && (
                                                <Tooltip title="View Details">
                                                    <IconButton
                                                        aria-label={`View details for ${provider.name}`}
                                                        size="small"
                                                        color="primary"
                                                        onClick={() => onEdit(provider.uuid)}
                                                    >
                                                        <Edit fontSize="small"/>
                                                    </IconButton>
                                                </Tooltip>
                                            )}
                                            <Divider orientation="vertical" flexItem/>
                                            {/* Quota text button */}
                                            {onQuotaRefresh && (
                                                <Button
                                                    variant="text"
                                                    size="small"
                                                    startIcon={
                                                        refreshingQuotas?.has(provider.uuid) ? (
                                                            <CircularProgress size={12}/>
                                                        ) : (
                                                            <DataUsage fontSize="small"/>
                                                        )
                                                    }
                                                    onClick={() => onQuotaRefresh(provider.uuid)}
                                                    disabled={refreshingQuotas?.has(provider.uuid)}
                                                    color="primary"
                                                    sx={{
                                                        minWidth: "auto",
                                                        px: {xs: 0.75, xl: 1},
                                                        '& .MuiButton-startIcon': {display: {xs: 'none', xl: 'inherit'}},
                                                    }}
                                                >
                                                    Quota
                                                </Button>
                                            )}
                                            {/* Models text button */}
                                            <Button
                                                variant="text"
                                                size="small"
                                                startIcon={<ListAlt/>}
                                                onClick={() => handleModelListClick(provider.uuid)}
                                                sx={{
                                                    fontSize: "0.75rem",
                                                    minWidth: "auto",
                                                    px: {xs: 0.75, xl: 1},
                                                    '& .MuiButton-startIcon': {display: {xs: 'none', xl: 'inherit'}},
                                                }}
                                            >
                                                Models
                                            </Button>
                                            <Divider orientation="vertical" flexItem/>
                                            {/* Overflow menu */}
                                            <IconButton
                                                aria-label={`More actions for ${provider.name}`}
                                                size="small"
                                                onClick={(e) => handleMoreOpen(e, provider.uuid)}
                                            >
                                                <MoreVert fontSize="small"/>
                                            </IconButton>
                                        </Box>
                                    </TableCell>
                                </TableRow>
                                {/* Quota detail row */}
                                {providerQuotas && onQuotaRefresh && (
                                    <ProviderQuotaDetailRow
                                        provider={provider}
                                        quota={providerQuotas[provider.uuid]}
                                        isRefreshing={refreshingQuotas?.has(provider.uuid) || false}
                                        onRefresh={onQuotaRefresh}
                                    />
                                )}
                            </React.Fragment>
                        );
                    })}
                </TableBody>
            </Table>
            {/* Overflow menu (shared) */}
            <Menu
                anchorEl={moreMenu.anchorEl}
                open={Boolean(moreMenu.anchorEl)}
                onClose={handleMoreClose}
                onClick={(e) => e.stopPropagation()}
                anchorOrigin={{vertical: "bottom", horizontal: "right"}}
                transformOrigin={{vertical: "top", horizontal: "right"}}
            >
                {(() => {
                    const p = providers.find((p) => p.uuid === moreMenu.providerUuid);
                    if (!p) return null;
                    const hasRefreshToken =
                        onRefreshToken && p.oauth_detail?.refresh_token;
                    const expired = p.oauth_detail?.expires_at
                        ? new Date(p.oauth_detail.expires_at) < new Date()
                        : false;
                    return [
                        hasRefreshToken && (
                            <MenuItem
                                key="refresh-token"
                                onClick={() => {
                                    handleMoreClose();
                                    handleRefreshClick(p.uuid);
                                }}
                                disabled={refreshing === p.uuid}
                            >
                                {refreshing === p.uuid ? (
                                    <CircularProgress size={14} sx={{mr: 1}}/>
                                ) : (
                                    <RefreshIcon fontSize="small" sx={{mr: 1}}/>
                                )}
                                Refresh Token
                            </MenuItem>
                        ),
                        onReauthorize && (
                            <MenuItem
                                key="reauthorize"
                                onClick={() => {
                                    handleMoreClose();
                                    onReauthorize(p.uuid);
                                }}
                                sx={{color: expired ? "warning.main" : undefined}}
                            >
                                <VpnKey fontSize="small" sx={{mr: 1}}/> Reauthorize
                            </MenuItem>
                        ),
                        <Divider key="div1"/>,
                        <MenuItem
                            key="copy-base64"
                            onClick={() => {
                                handleMoreClose();
                                handleCopyProviderBase64(p);
                            }}
                        >
                            <ContentCopy fontSize="small" sx={{mr: 1}}/> Copy Base64
                        </MenuItem>,
                        <MenuItem
                            key="copy-jsonl"
                            onClick={() => {
                                handleMoreClose();
                                handleCopyProviderJsonl(p);
                            }}
                        >
                            <ContentCopy fontSize="small" sx={{mr: 1}}/> Copy JSONL
                        </MenuItem>,
                        onDelete && <Divider key="div2"/>,
                        onDelete && (
                            <MenuItem
                                key="delete"
                                onClick={() => {
                                    handleMoreClose();
                                    handleDeleteClick(p.uuid);
                                }}
                                sx={{color: "error.main"}}
                            >
                                <Delete fontSize="small" sx={{mr: 1}}/> Delete
                            </MenuItem>
                        ),
                    ].filter(Boolean);
                })()}
            </Menu>
            {/* Delete Confirmation Modal */}
            <Modal open={deleteModal.open} onClose={handleCloseDeleteModal}>
                <Box
                    sx={{
                        position: "absolute",
                        top: "50%",
                        left: "50%",
                        transform: "translate(-50%, -50%)",
                        width: 400,
                        maxWidth: "80vw",
                        bgcolor: "background.paper",
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{mb: 2}}>
                        Delete OAuth Provider
                    </Typography>
                    <Typography variant="body2" sx={{mb: 3}}>
                        Are you sure you want to delete the OAuth provider "
                        {deleteModal.providerName}"? This action cannot be undone.
                    </Typography>
                    <Stack direction="row" spacing={2} sx={{
                        justifyContent: "flex-end"
                    }}>
                        <Button onClick={handleCloseDeleteModal} color="inherit">
                            Cancel
                        </Button>
                        <Button
                            onClick={handleConfirmDelete}
                            color="error"
                            variant="contained"
                        >
                            Delete
                        </Button>
                    </Stack>
                </Box>
            </Modal>
            {/* Refresh Token Confirmation Modal */}
            <Modal open={refreshModal.open} onClose={handleCloseRefreshModal}>
                <Box
                    sx={{
                        position: "absolute",
                        top: "50%",
                        left: "50%",
                        transform: "translate(-50%, -50%)",
                        width: 400,
                        maxWidth: "80vw",
                        bgcolor: "background.paper",
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{mb: 2}}>
                        Refresh OAuth Token
                    </Typography>
                    <Typography variant="body2" sx={{mb: 3}}>
                        Are you sure you want to refresh the OAuth token for "
                        {refreshModal.providerName}"? This will update the access token
                        using the refresh token.
                    </Typography>
                    <Stack direction="row" spacing={2} sx={{
                        justifyContent: "flex-end"
                    }}>
                        <Button
                            onClick={handleCloseRefreshModal}
                            color="inherit"
                            disabled={refreshing !== null}
                        >
                            Cancel
                        </Button>
                        <Button
                            onClick={handleConfirmRefresh}
                            color="info"
                            variant="contained"
                            disabled={refreshing !== null}
                            startIcon={
                                refreshing !== null ? (
                                    <CircularProgress size={16}/>
                                ) : (
                                    <RefreshIcon fontSize="small"/>
                                )
                            }
                        >
                            {refreshing !== null ? "Refreshing..." : "Refresh"}
                        </Button>
                    </Stack>
                </Box>
            </Modal>
            {/* Model List Dialog */}
            <ModelListDialog
                open={modelListDialog.open}
                onClose={handleCloseModelListDialog}
                provider={modelListDialog.provider}
            />
        </TableContainer>
    );
};

export default OAuthTable;
