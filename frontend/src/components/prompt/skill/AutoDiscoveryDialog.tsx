import {
    CheckCircle,
    Close,
    Download,
    Search,
    WarningAmber,
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    Checkbox,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    LinearProgress,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { type SkillLocation, type DiscoveryResult } from '@/types/prompt';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';

interface AutoDiscoveryDialogProps {
    open: boolean;
    onClose: () => void;
    onImport: (locations: SkillLocation[]) => Promise<void>;
}

interface DiscoveredLocation extends SkillLocation {
    selected: boolean;
}

const AutoDiscoveryDialog = ({ open, onClose, onImport }: AutoDiscoveryDialogProps) => {
    const [discovering, setDiscovering] = useState(false);
    const [importing, setImporting] = useState(false);
    const [discoveryResult, setDiscoveryResult] = useState<DiscoveryResult | null>(null);
    const [discoveredLocations, setDiscoveredLocations] = useState<DiscoveredLocation[]>([]);
    const [searchQuery, setSearchQuery] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [selectAll, setSelectAll] = useState(false);

    useEffect(() => {
        if (open) {
            handleDiscover();
        } else {
            // Reset state when dialog closes
            setDiscoveryResult(null);
            setDiscoveredLocations([]);
            setSearchQuery('');
            setError(null);
            setSelectAll(false);
        }
    }, [open]);

    useEffect(() => {
        const allSelected =
            discoveredLocations.length > 0 &&
            discoveredLocations.every((loc) => loc.selected);
        setSelectAll(allSelected);
    }, [discoveredLocations]);

    const handleDiscover = async () => {
        setDiscovering(true);
        setError(null);
        setDiscoveryResult(null);
        setDiscoveredLocations([]);

        try {
            const result = await api.discoverIdes();
            if (result.success && result.data) {
                setDiscoveryResult(result.data);
                // Initialize all locations as selected
                const locations = result.data.locations.map((loc: SkillLocation) => ({
                    ...loc,
                    selected: true,
                }));
                setDiscoveredLocations(locations);
            } else {
                setError(result.error || 'Failed to discover IDEs');
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Unknown error occurred');
        } finally {
            setDiscovering(false);
        }
    };

    const handleToggleSelect = (id: string) => {
        setDiscoveredLocations((prev) =>
            prev.map((loc) =>
                loc.id === id ? { ...loc, selected: !loc.selected } : loc
            )
        );
    };

    const handleToggleSelectAll = () => {
        const newValue = !selectAll;
        setSelectAll(newValue);
        setDiscoveredLocations((prev) =>
            prev.map((loc) => ({ ...loc, selected: newValue }))
        );
    };

    const handleImport = async () => {
        const selectedLocations = discoveredLocations
            .filter((loc) => loc.selected)
            .map(({ selected, ...loc }) => loc);

        if (selectedLocations.length === 0) {
            setError('Please select at least one location to import');
            return;
        }

        setImporting(true);
        setError(null);

        try {
            await onImport(selectedLocations);
            handleClose();
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to import locations');
        } finally {
            setImporting(false);
        }
    };

    const handleClose = () => {
        if (!discovering && !importing) {
            onClose();
        }
    };

    const filteredLocations = discoveredLocations.filter((loc) =>
        loc.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        loc.path.toLowerCase().includes(searchQuery.toLowerCase())
    );

    const selectedCount = discoveredLocations.filter((loc) => loc.selected).length;

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
            <DialogTitle>
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                    }}
                >
                    <Typography variant="h6">Auto Discover Skills</Typography>
                    <IconButton
                        aria-label="close"
                        onClick={handleClose}
                        size="small"
                        disabled={discovering || importing}
                    >
                        <Close />
                    </IconButton>
                </Box>
            </DialogTitle>
            <DialogContent sx={{ pb: 1 }}>
                <Stack spacing={2}>
                    {/* Discovery Summary */}
                    {discoveryResult && (
                        <Alert
                            severity={
                                discoveryResult.locations.length > 0 ? 'success' : 'info'
                            }
                            icon={<CheckCircle fontSize="inherit" />}
                        >
                            <Typography variant="body2">
                                <strong>Discovery Complete</strong>
                                <br />
                                Scanned {discoveryResult.total_ides_scanned} IDE(s), found{' '}
                                {discoveryResult.ides_found.length} installed, discovered{' '}
                                {discoveryResult.locations.length} skill location(s) with{' '}
                                {discoveryResult.skills_found} total skill(s).
                            </Typography>
                        </Alert>
                    )}

                    {/* Error Alert */}
                    {error && (
                        <Alert
                            severity="error"
                            icon={<WarningAmber fontSize="inherit" />}
                            onClose={() => setError(null)}
                        >
                            {error}
                        </Alert>
                    )}

                    {/* Search and Filter */}
                    {!discovering && discoveryResult && (
                        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
                            <TextField
                                placeholder="Search locations..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                InputProps={{
                                    startAdornment: (
                                        <Search sx={{ mr: 1, color: 'text.secondary' }} />
                                    ),
                                }}
                                size="small"
                                fullWidth
                            />
                            <Button
                                variant="outlined"
                                size="small"
                                onClick={handleDiscover}
                                disabled={discovering}
                            >
                                Refresh
                            </Button>
                        </Box>
                    )}

                    {/* Progress */}
                    {discovering && (
                        <Box sx={{ textAlign: 'center', py: 3 }}>
                            <CircularProgress size={40} sx={{ mb: 2 }} />
                            <Typography variant="body2" color="text.secondary">
                                Scanning home directory for IDE installations...
                            </Typography>
                            <LinearProgress sx={{ mt: 2 }} />
                        </Box>
                    )}

                    {/* Location List */}
                    {!discovering && filteredLocations.length > 0 && (
                        <Box>
                            <Box
                                sx={{
                                    display: 'flex',
                                    justifyContent: 'space-between',
                                    alignItems: 'center',
                                    mb: 1,
                                }}
                            >
                                <Typography variant="subtitle2" color="text.secondary">
                                    {selectedCount} of {filteredLocations.length} selected
                                </Typography>
                                <Button
                                    size="small"
                                    onClick={handleToggleSelectAll}
                                    disabled={filteredLocations.length === 0}
                                >
                                    {selectAll ? 'Deselect All' : 'Select All'}
                                </Button>
                            </Box>
                            <Stack spacing={1.5}>
                                {filteredLocations.map((location) => (
                                    <Card
                                        key={location.id}
                                        sx={{
                                            border: 1,
                                            borderColor: location.selected
                                                ? 'primary.main'
                                                : 'divider',
                                            bgcolor: location.selected
                                                ? 'primary.50'
                                                : 'background.paper',
                                        }}
                                    >
                                        <CardContent
                                            sx={{
                                                py: 1.5,
                                                px: 2,
                                                display: 'flex',
                                                alignItems: 'center',
                                                gap: 2,
                                            }}
                                        >
                                            <Checkbox
                                                checked={location.selected}
                                                onChange={() => handleToggleSelect(location.id)}
                                            />
                                            <Box
                                                sx={{
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    gap: 1,
                                                    flex: 1,
                                                    minWidth: 0,
                                                }}
                                            >
                                                <Chip
                                                    size="small"
                                                    label={getIdeSourceLabel(location.ide_source)}
                                                    variant="outlined"
                                                    sx={{ height: 24, fontSize: '0.75rem' }}
                                                />
                                                <Box sx={{ minWidth: 0, flex: 1 }}>
                                                    <Typography
                                                        variant="subtitle2"
                                                        sx={{ fontWeight: 600 }}
                                                    >
                                                        {location.name}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                        sx={{
                                                            overflow: 'hidden',
                                                            textOverflow: 'ellipsis',
                                                            whiteSpace: 'nowrap',
                                                            display: 'block',
                                                        }}
                                                    >
                                                        {location.path}
                                                    </Typography>
                                                </Box>
                                            </Box>
                                            <Typography
                                                variant="caption"
                                                color="text.secondary"
                                                sx={{ whiteSpace: 'nowrap' }}
                                            >
                                                {location.skill_count} skill
                                                {location.skill_count !== 1 ? 's' : ''}
                                            </Typography>
                                        </CardContent>
                                    </Card>
                                ))}
                            </Stack>
                        </Box>
                    )}

                    {/* No Results */}
                    {!discovering && !error && filteredLocations.length === 0 && searchQuery && (
                        <Box sx={{ textAlign: 'center', py: 3 }}>
                            <Typography variant="body2" color="text.secondary">
                                No locations match your search.
                            </Typography>
                        </Box>
                    )}

                    {!discovering && !error && discoveryResult && discoveryResult.locations.length === 0 && (
                        <Box sx={{ textAlign: 'center', py: 3 }}>
                            <Alert severity="info">
                                <Typography variant="body2">
                                    No skill locations were discovered. This could mean:
                                    <br />
                                    • No supported IDEs are installed
                                    <br />
                                    • No skill directories exist in the default locations
                                    <br />
                                    • Try adding a location manually
                                </Typography>
                            </Alert>
                        </Box>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={handleClose} size="small" disabled={discovering || importing}>
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    size="small"
                    onClick={handleImport}
                    disabled={discovering || importing || selectedCount === 0}
                    startIcon={importing ? <CircularProgress size={16} /> : <Download />}
                >
                    {importing
                        ? `Importing (${selectedCount})...`
                        : `Import ${selectedCount} Location${selectedCount !== 1 ? 's' : ''}`}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AutoDiscoveryDialog;
