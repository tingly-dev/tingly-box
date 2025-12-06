import {
    Box,
    Button,
    Chip,
    FormControl,
    FormControlLabel,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';

interface HistoryFiltersProps {
    searchTerm: string;
    onSearchChange: (value: string) => void;
    filterType: string;
    onFilterTypeChange: (value: string) => void;
    filterStatus: string;
    onFilterStatusChange: (value: string) => void;
    onRefresh: () => void;
    onExport: () => void;
    autoRefresh: boolean;
    onAutoRefreshChange: (enabled: boolean) => void;
    refreshInterval: number;
    onRefreshIntervalChange: (interval: number) => void;
}

const HistoryFilters = ({
    searchTerm,
    onSearchChange,
    filterType,
    onFilterTypeChange,
    filterStatus,
    onFilterStatusChange,
    onRefresh,
    onExport,
    autoRefresh,
    onAutoRefreshChange,
    refreshInterval,
    onRefreshIntervalChange,
}: HistoryFiltersProps) => {
    return (
        <Stack spacing={3}>
            <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                <TextField
                    fullWidth
                    label="Search history..."
                    value={searchTerm}
                    onChange={(e) => onSearchChange(e.target.value)}
                />
                <FormControl fullWidth>
                    <InputLabel>Filter by Action</InputLabel>
                    <Select
                        value={filterType}
                        onChange={(e) => onFilterTypeChange(e.target.value)}
                        label="Filter by Action"
                    >
                        <MenuItem value="all">All Actions</MenuItem>
                        <MenuItem value="start_server">Start Server</MenuItem>
                        <MenuItem value="stop_server">Stop Server</MenuItem>
                        <MenuItem value="restart_server">Restart Server</MenuItem>
                        <MenuItem value="add_provider">Add Provider</MenuItem>
                        <MenuItem value="delete_provider">Delete Provider</MenuItem>
                        <MenuItem value="generate_token">Generate Token</MenuItem>
                    </Select>
                </FormControl>
                <FormControl fullWidth>
                    <InputLabel>Filter by Status</InputLabel>
                    <Select
                        value={filterStatus}
                        onChange={(e) => onFilterStatusChange(e.target.value)}
                        label="Filter by Status"
                    >
                        <MenuItem value="all">All Status</MenuItem>
                        <MenuItem value="true">Success</MenuItem>
                        <MenuItem value="false">Error</MenuItem>
                    </Select>
                </FormControl>
            </Stack>

            <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems="center">
                <Stack direction="row" spacing={2}>
                    <Button variant="outlined" onClick={onRefresh}>
                        Refresh
                    </Button>
                    <Button variant="contained" onClick={onExport}>
                        Export
                    </Button>
                </Stack>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, ml: 'auto' }}>
                    <FormControlLabel
                        control={
                            <Switch
                                checked={autoRefresh}
                                onChange={(e) => onAutoRefreshChange(e.target.checked)}
                                color="primary"
                            />
                        }
                        label="Auto Refresh"
                    />
                    <FormControl size="small" sx={{ minWidth: 120 }}>
                        <InputLabel>Interval</InputLabel>
                        <Select
                            value={refreshInterval}
                            onChange={(e) => onRefreshIntervalChange(Number(e.target.value))}
                            label="Interval"
                            disabled={!autoRefresh}
                        >
                            <MenuItem value={10000}>10s</MenuItem>
                            <MenuItem value={30000}>30s</MenuItem>
                            <MenuItem value={60000}>1m</MenuItem>
                            <MenuItem value={300000}>5m</MenuItem>
                        </Select>
                    </FormControl>
                    {autoRefresh && (
                        <Chip
                            label="Active"
                            color="success"
                            size="small"
                            variant="outlined"
                        />
                    )}
                </Box>
            </Stack>
        </Stack>
    );
};

export default HistoryFilters;