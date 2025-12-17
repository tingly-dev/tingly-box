import {
    Api as ApiIcon,
    Delete as DeleteIcon,
    ExpandLess as ExpandLessIcon,
    ExpandMore as ExpandMoreIcon,
    Save as SaveIcon
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
    Typography
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
    onUpdateRecord: (field: keyof ConfigRecord, value: any) => void;
    onUpdateProvider: (providerId: string, field: keyof ConfigProvider, value: any) => void;
    onAddProvider: () => void;
    onDeleteProvider: (providerId: string) => void;
    onRefreshModels: (providerName: string) => void;
    onSave: () => void;
    onDelete: () => void;
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
    onUpdateRecord,
    onUpdateProvider,
    onAddProvider,
    onDeleteProvider,
    onRefreshModels,
    onSave,
    onDelete
}) => {
    const [expanded, setExpanded] = useState(false);

    return (
        <StyledCard active={record.active}>
            <SummarySection onClick={() => setExpanded(!expanded)}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        {record.requestModel || 'Untitled Rule'}
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
                            startIcon={<DeleteIcon />}
                            onClick={(e) => {
                                e.stopPropagation();
                                onDelete();
                            }}
                            variant="outlined"
                            size="small"
                            color="error"
                            disabled={saving}
                            sx={{ minWidth: 'auto', px: 1.5 }}
                        >
                            Delete
                        </Button>
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
                            setExpanded(!expanded);
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
                                Model Configuration
                            </Typography>
                            <Stack direction="row" spacing={1}>
                                <TextField
                                    label="Request Model"
                                    value={record.requestModel}
                                    onChange={(e) => onUpdateRecord('requestModel', e.target.value)}
                                    helperText="Model name to match"
                                    size="small"
                                    disabled={!record.active}
                                />
                                <TextField
                                    label="Response Model"
                                    value={record.responseModel}
                                    onChange={(e) => onUpdateRecord('responseModel', e.target.value)}
                                    helperText="Empty for as-is"
                                    size="small"
                                    disabled={!record.active}
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