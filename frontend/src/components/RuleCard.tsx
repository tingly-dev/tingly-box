import {
    Delete as DeleteIcon,
    ExpandLess as ExpandLessIcon,
    ExpandMore as ExpandMoreIcon,
    Save as SaveIcon,
    MoreVert as MoreVertIcon,
    Settings as SettingsIcon,
    Refresh as RefreshIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    Stack,
    Switch,
    TextField,
    Typography,
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import ProviderConfig from './ProviderConfig';

interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
}

interface ConfigRecord {
    uuid: string;
    requestModel: string;
    responseModel: string;
    active: boolean;
    providers: ConfigProvider[];
}

interface RuleCardProps {
    record: ConfigRecord;
    providers: any[];
    providerModels: any;
    saving: boolean;
    expanded: boolean;
    onUpdateRecord: (field: keyof ConfigRecord, value: any) => void;
    onUpdateProvider: (providerId: string, field: keyof ConfigProvider, value: any) => void;
    onAddProvider: () => void;
    onDeleteProvider: (providerId: string) => void;
    onRefreshModels: (providerName: string) => void;
    onSave: () => void;
    onDelete: () => void;
    onReset: () => void;
    onToggleExpanded: () => void;
}

const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active }) => ({
    transition: 'all 0.2s ease-in-out',
}));

const SummarySection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: theme.spacing(2),
    cursor: 'pointer',
    '&:hover': {
        backgroundColor: 'action.hover',
    },
}));

const RuleCard: React.FC<RuleCardProps> = ({
                                               record,
                                               providers,
                                               providerModels,
                                               saving,
                                               expanded,
                                               onUpdateRecord,
                                               onUpdateProvider,
                                               onAddProvider,
                                               onDeleteProvider,
                                               onRefreshModels,
                                               onSave,
                                               onDelete,
                                               onReset,
                                               onToggleExpanded
                                           }) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [showResponseField, setShowResponseField] = useState(false);
    const menuOpen = Boolean(menuAnchorEl);

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setMenuAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setMenuAnchorEl(null);
    };

    const handleConfigureResponse = () => {
        handleMenuClose();
        // TODO: Add configure response logic
        console.log('Configure response for:', record.responseModel);
    };

    const handleConfigureResponseModel = () => {
        handleMenuClose();
        // Show the response field and expand the card
        setShowResponseField(true);
        if (!expanded) {
            onToggleExpanded();
        }
        // Use a timeout to ensure the field is rendered before focusing
        setTimeout(() => {
            const responseField = document.getElementById(`response-model-${record.uuid}`) as HTMLInputElement;
            if (responseField) {
                responseField.focus();
                responseField.select();
            }
        }, 100);
    };

    return (
        <StyledCard active={record.active}>
            <SummarySection onClick={onToggleExpanded}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        {record.requestModel || 'Specified model name'}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Chip
                            // icon={<ApiIcon sx={{ fontSize: 16 }} />}
                            label={`use ${record.providers.length} keys`}
                            size="small"
                            variant="outlined"
                        />
                        <Chip
                            label={record.active ? 'Active' : 'Inactive'}
                            color={record.active ? 'success' : 'default'}
                            size="small"
                        />
                        {
                            record.responseModel && <Chip
                                label={`Response as ${record.responseModel}`}
                                size="small"
                                color={'info'}
                            />
                        }
                    </Box>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    {/* Action Buttons */}
                    <Box sx={{ display: 'flex', gap: 0.5, mr: 1 }}>
                        <Button
                            variant="contained"
                            color="primary"
                            size="small"
                            startIcon={<SaveIcon />}
                            onClick={(e) => {
                                e.stopPropagation();
                                onSave();
                            }}
                            disabled={saving}
                            sx={{ minWidth: 'auto', px: 1.5 }}
                        >
                            {saving ? 'Saving...' : 'Save'}
                        </Button>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={(e) => {
                                e.stopPropagation();
                                onReset();
                            }}
                            variant="outlined"
                            size="small"
                            disabled={saving}
                            sx={{ minWidth: 'auto', px: 1.5 }}
                        >
                            Reset
                        </Button>
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            disabled={saving}
                            sx={{ ml: 0.5 }}
                        >
                            <MoreVertIcon />
                        </IconButton>
                        <Menu
                            anchorEl={menuAnchorEl}
                            open={menuOpen}
                            onClose={handleMenuClose}
                            onClick={(e) => e.stopPropagation()}
                        >
                            {!record.responseModel ? (
                                <MenuItem onClick={handleConfigureResponseModel}>
                                    <ListItemIcon>
                                        <SettingsIcon fontSize="small" />
                                    </ListItemIcon>
                                    <ListItemText>Configure Response Model</ListItemText>
                                </MenuItem>
                            ) : (
                                <MenuItem onClick={handleConfigureResponse}>
                                    <ListItemIcon>
                                        <SettingsIcon fontSize="small" />
                                    </ListItemIcon>
                                    <ListItemText>Configure Response</ListItemText>
                                </MenuItem>
                            )}
                            <MenuItem onClick={(e) => {
                                e.stopPropagation();
                                onDelete();
                            }}>

                                <ListItemIcon>
                                    <DeleteIcon fontSize="small" />
                                </ListItemIcon>
                                <ListItemText>Delete Rule</ListItemText>
                            </MenuItem>
                        </Menu>
                    </Box>
                    <Switch
                        checked={record.active}
                        onChange={(e) => onUpdateRecord('active', e.target.checked)}
                        size="small"
                        disabled={saving}
                        onClick={(e) => e.stopPropagation()}
                    />
                    <IconButton
                        size="small"
                        onClick={(e) => {
                            e.stopPropagation();
                            onToggleExpanded();
                        }}
                    >
                        {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                    </IconButton>
                </Box>
            </SummarySection>

            <Collapse in={expanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0 }}>
                    <Stack spacing={2}>
                        {/* Model Configuration */}
                        <Box>
                            <Typography variant="subtitle1" sx={{ fontWeight: 600, color: 'text.primary', mb: 1.5 }}>
                                Local Model
                            </Typography>
                            <Stack direction="row" spacing={1}>
                                <TextField
                                    id={`request-model-${record.uuid}`}
                                    label="Request Model"
                                    value={record.requestModel}
                                    onChange={(e) => onUpdateRecord('requestModel', e.target.value)}
                                    helperText="Enter specified model name"
                                    size="small"
                                    disabled={!record.active}
                                    fullWidth
                                />
                                <TextField
                                    id={`response-model-${record.uuid}`}
                                    label="Response Model"
                                    value={record.responseModel}
                                    onChange={(e) => {
                                        onUpdateRecord('responseModel', e.target.value);
                                        if (e.target.value) {
                                            setShowResponseField(false);
                                        }
                                    }}
                                    helperText={record.responseModel ? "Model to return as" : "Leave empty for as-is response"}
                                    size="small"
                                    disabled={!record.active}
                                    sx={{
                                        minWidth: 200,
                                        display: record.responseModel || showResponseField ? 'block' : 'none'
                                    }}
                                />
                            </Stack>
                        </Box>

                        {/* Provider Configuration */}
                        <ProviderConfig
                            providers={record.providers}
                            availableProviders={providers}
                            providerModels={providerModels}
                            active={record.active}
                            onAddProvider={onAddProvider}
                            onDeleteProvider={onDeleteProvider}
                            onUpdateProvider={onUpdateProvider}
                            onRefreshModels={onRefreshModels}
                        />
                    </Stack>
                </CardContent>
            </Collapse>
        </StyledCard>
    );
};

export default RuleCard;