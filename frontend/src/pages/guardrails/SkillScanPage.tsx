import {
    Add,
    AutoFixHigh,
    Code,
    ContentCopy,
    Description,
    FolderOpen,
    Refresh,
    Search,
    Visibility,
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    IconButton,
    InputAdornment,
    LinearProgress,
    List,
    ListItem,
    ListItemButton,
    ListItemText,
    Paper,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
    Chip as MuiChip,
} from '@mui/material';
import { useEffect, useState } from 'react';
import XMarkdown from '@ant-design/x-markdown';
import { type SkillLocation, type Skill, type IDESource } from '@/types/prompt';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { getIdeSourceLabel } from '@/constants/ideSources';
import { api } from '@/services/api';
import AddSkillLocationDialog from '@/components/prompt/skill/AddSkillLocationDialog';
import AutoDiscoveryDialog from '@/components/prompt/skill/AutoDiscoveryDialog';

interface AddSkillLocationData {
    name: string;
    path: string;
    ide_source: IDESource;
}

type SourceScanStatus = 'idle' | 'queued' | 'scanning' | 'done' | 'failed';
type SkillScanTab = 'overview' | 'skills' | 'findings';
type FindingSeverity = 'critical' | 'high' | 'medium' | 'low';

interface SourceScanState {
    status: SourceScanStatus;
    progress: number;
    scannedSkills?: number;
    durationMs?: number;
    lastScannedAt?: number;
    error?: string;
}

interface ScanRunState {
    active: boolean;
    total: number;
    completed: number;
    currentLocationId?: string;
    startedAt?: number;
    finishedAt?: number;
}

interface SkillScanFinding {
    id: string;
    severity: FindingSeverity;
    tag: string;
    skillName: string;
    sourceName: string;
    filePath: string;
    line: number;
    snippet: string;
}

const SkillScanPage = () => {
    const [locations, setLocations] = useState<SkillLocation[]>([]);
    const [loading, setLoading] = useState(true);
    const [notification, setNotification] = useState<{
        open: boolean;
        message: string;
        severity: 'success' | 'error';
    }>({ open: false, message: '', severity: 'success' });

    // Location list state
    const [locationSearch, setLocationSearch] = useState('');
    const [selectedLocation, setSelectedLocation] = useState<SkillLocation | null>(null);

    // Skill list state
    const [skills, setSkills] = useState<Skill[]>([]);
    const [skillsLoading, setSkillsLoading] = useState(false);
    const [skillSearch, setSkillSearch] = useState('');
    const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);

    // Skill detail state
    const [skillContent, setSkillContent] = useState<string>('');
    const [contentLoading, setContentLoading] = useState(false);
    const [viewMode, setViewMode] = useState<'markdown' | 'raw'>('raw');
    const [activeTab, setActiveTab] = useState<SkillScanTab>('overview');
    const [findingSearch, setFindingSearch] = useState('');
    const [findingSeverity, setFindingSeverity] = useState<'all' | FindingSeverity>('all');
    const [selectedFindingId, setSelectedFindingId] = useState<string | null>(null);

    // Dialog states
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const [discoveryDialogOpen, setDiscoveryDialogOpen] = useState(false);
    const [sourceScanStates, setSourceScanStates] = useState<Record<string, SourceScanState>>({});
    const [scanRun, setScanRun] = useState<ScanRunState>({
        active: false,
        total: 0,
        completed: 0,
    });

    useEffect(() => {
        loadLocations();
    }, []);

    // Load skills when location is selected
    useEffect(() => {
        if (selectedLocation) {
            loadSkills(selectedLocation);
        } else {
            setSkills([]);
            setSelectedSkill(null);
            setSkillContent('');
        }
    }, [selectedLocation]);

    // Load skill content when skill is selected
    useEffect(() => {
        if (selectedSkill && selectedLocation) {
            loadSkillContent(selectedSkill);
        } else {
            setSkillContent('');
            setViewMode('raw');
        }
    }, [selectedSkill]);

    const showNotification = (message: string, severity: 'success' | 'error') => {
        setNotification({ open: true, message, severity });
    };

    const loadLocations = async () => {
        setLoading(true);
        const result = await api.getSkillLocations();
        if (result.success) {
            const nextLocations = result.data || [];
            setLocations(nextLocations);
            setSourceScanStates(prev => {
                const next: Record<string, SourceScanState> = {};
                nextLocations.forEach((location: SkillLocation) => {
                    next[location.id] = prev[location.id] || {
                        status: 'idle',
                        progress: 0,
                    };
                });
                return next;
            });
        } else {
            showNotification(`Failed to load locations: ${result.error}`, 'error');
        }
        setLoading(false);
    };

    const loadSkills = async (location: SkillLocation) => {
        setSkillsLoading(true);
        const result = await api.refreshSkillLocation(location.id);
        if (result.success && result.data) {
            setSkills(result.data.skills || []);
            // Update the location's skill count in the locations list
            setLocations(prev =>
                prev.map(loc =>
                    loc.id === location.id
                        ? { ...loc, skill_count: result.data.skills?.length || 0 }
                        : loc
                )
            );
        } else {
            showNotification(`Failed to load skills: ${result.error}`, 'error');
        }
        setSkillsLoading(false);
    };

    const loadSkillContent = async (skill: Skill) => {
        if (!selectedLocation) return;

        setContentLoading(true);
        const result = await api.getSkillContent(
            selectedLocation.id,
            skill.id,
            skill.entry_path || skill.path
        );
        if (result.success && result.data) {
            setSkillContent(result.data.content || '');
        } else {
            showNotification(`Failed to load skill content: ${result.error}`, 'error');
        }
        setContentLoading(false);
    };

    const handleAddClick = () => {
        setAddDialogOpen(true);
    };

    const handleAddSubmit = async (data: AddSkillLocationData) => {
        const result = await api.addSkillLocation({
            name: data.name,
            path: data.path,
            ide_source: data.ide_source,
        });
        if (result.success) {
            showNotification('Location added successfully!', 'success');
            loadLocations();
        } else {
            showNotification(`Failed to add location: ${result.error}`, 'error');
        }
    };

    const handleImportLocations = async (locs: SkillLocation[]) => {
        const result = await api.importSkillLocations(locs);
        if (result.success) {
            showNotification(
                `Imported ${result.data?.length || 0} location(s) successfully!`,
                'success'
            );
            loadLocations();
        } else {
            showNotification(`Failed to import locations: ${result.error}`, 'error');
        }
    };

    const handleCopyContent = () => {
        navigator.clipboard.writeText(skillContent);
        showNotification('Copied to clipboard!', 'success');
    };

    const updateLocationSkillCount = (locationId: string, count: number) => {
        setLocations(prev =>
            prev.map(loc =>
                loc.id === locationId
                    ? { ...loc, skill_count: count }
                    : loc
            )
        );
    };

    const scanSingleLocation = async (location: SkillLocation, notify = false): Promise<boolean> => {
        const startedAt = Date.now();
        let progress = 8;
        setSourceScanStates(prev => ({
            ...prev,
            [location.id]: {
                ...(prev[location.id] || { status: 'idle', progress: 0 }),
                status: 'scanning',
                progress,
                error: undefined,
            },
        }));

        const timer = window.setInterval(() => {
            progress = Math.min(progress + 7, 88);
            setSourceScanStates(prev => ({
                ...prev,
                [location.id]: {
                    ...(prev[location.id] || { status: 'scanning', progress }),
                    status: 'scanning',
                    progress,
                },
            }));
        }, 160);

        try {
            const result = await api.refreshSkillLocation(location.id);
            window.clearInterval(timer);

            if (result.success && result.data) {
                const scannedSkills = result.data.skills || [];
                updateLocationSkillCount(location.id, scannedSkills.length);

                if (selectedLocation?.id === location.id) {
                    setSkills(scannedSkills);
                }

                setSourceScanStates(prev => ({
                    ...prev,
                    [location.id]: {
                        status: 'done',
                        progress: 100,
                        scannedSkills: scannedSkills.length,
                        durationMs: Date.now() - startedAt,
                        lastScannedAt: Date.now(),
                    },
                }));

                if (notify) {
                    showNotification('Location scanned successfully!', 'success');
                }
                return true;
            }

                setSourceScanStates(prev => ({
                    ...prev,
                    [location.id]: {
                        status: 'failed',
                        progress: 100,
                        error: result.error || 'Scan failed',
                        durationMs: Date.now() - startedAt,
                        lastScannedAt: Date.now(),
                    },
                }));

            if (notify) {
                showNotification(`Failed to scan location: ${result.error}`, 'error');
            }
            return false;
        } catch (error) {
            window.clearInterval(timer);
            const message = error instanceof Error ? error.message : 'Scan failed';
            setSourceScanStates(prev => ({
                ...prev,
                [location.id]: {
                    status: 'failed',
                    progress: 100,
                    error: message,
                    durationMs: Date.now() - startedAt,
                    lastScannedAt: Date.now(),
                },
            }));
            if (notify) {
                showNotification(`Failed to scan location: ${message}`, 'error');
            }
            return false;
        }
    };

    const handleScanAll = async () => {
        if (scanRun.active) {
            return;
        }

        if (locations.length === 0) {
            showNotification('Add or discover at least one location first.', 'error');
            return;
        }

        const targets = [...locations];
        setSourceScanStates(prev => {
            const next = { ...prev };
            targets.forEach((location) => {
                next[location.id] = {
                    ...(prev[location.id] || { progress: 0 }),
                    status: 'queued',
                    progress: 0,
                    error: undefined,
                };
            });
            return next;
        });
        setScanRun({
            active: true,
            total: targets.length,
            completed: 0,
            currentLocationId: targets[0]?.id,
            startedAt: Date.now(),
        });

        let completed = 0;
        for (const location of targets) {
            setScanRun(prev => ({
                ...prev,
                active: true,
                total: targets.length,
                completed,
                currentLocationId: location.id,
                startedAt: prev.startedAt || Date.now(),
            }));
            await scanSingleLocation(location, false);
            completed += 1;
            setScanRun(prev => ({
                ...prev,
                active: true,
                total: targets.length,
                completed,
                currentLocationId: location.id,
                startedAt: prev.startedAt || Date.now(),
            }));
        }

        setScanRun(prev => ({
            ...prev,
            active: false,
            total: targets.length,
            completed,
            currentLocationId: undefined,
            finishedAt: Date.now(),
        }));
        showNotification(`Scan complete: ${completed} source(s) processed.`, 'success');
    };

    // Filter locations
    const filteredLocations = locations.filter((location) => {
        const matchesSearch =
            locationSearch === '' ||
            location.name.toLowerCase().includes(locationSearch.toLowerCase()) ||
            location.path.toLowerCase().includes(locationSearch.toLowerCase());
        return matchesSearch;
    }).sort((a, b) => {
        // Stable sort: first by IDE source, then by name
        const aSource = getIdeSourceLabel(a.ide_source);
        const bSource = getIdeSourceLabel(b.ide_source);
        if (aSource !== bSource) {
            return aSource.localeCompare(bSource);
        }
        return a.name.localeCompare(b.name);
    });

    // Filter skills
    const filteredSkills = skills.filter((skill) => {
        const matchesSearch =
            skillSearch === '' ||
            skill.name.toLowerCase().includes(skillSearch.toLowerCase()) ||
            skill.filename.toLowerCase().includes(skillSearch.toLowerCase()) ||
            skill.path.toLowerCase().includes(skillSearch.toLowerCase());
        return matchesSearch;
    });

    const formatFileSize = (bytes?: number): string => {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    };

    const formatRelativeTime = (value?: number | Date) => {
        if (!value) return 'Not scanned yet';
        const date = value instanceof Date ? value : new Date(value);
        if (Number.isNaN(date.getTime())) return 'Not scanned yet';
        return date.toLocaleString();
    };

    const getSourceStatusMeta = (status: SourceScanStatus) => {
        switch (status) {
            case 'scanning':
                return { label: 'Scanning', color: 'warning' as const };
            case 'queued':
                return { label: 'Queued', color: 'default' as const };
            case 'done':
                return { label: 'Scanned', color: 'success' as const };
            case 'failed':
                return { label: 'Failed', color: 'error' as const };
            default:
                return { label: 'Ready', color: 'default' as const };
        }
    };

    const currentSourceState = scanRun.currentLocationId ? sourceScanStates[scanRun.currentLocationId] : undefined;
    const globalProgress = scanRun.total > 0
        ? ((scanRun.completed + ((currentSourceState?.status === 'scanning' ? (currentSourceState.progress / 100) : 0))) / scanRun.total) * 100
        : 0;
    const currentLocationName = scanRun.currentLocationId
        ? locations.find(location => location.id === scanRun.currentLocationId)?.name
        : undefined;
    const completedSources = Object.values(sourceScanStates).filter(state => state.status === 'done').length;
    const failedSources = Object.values(sourceScanStates).filter(state => state.status === 'failed').length;
    const activeSources = Object.values(sourceScanStates).filter(state => state.status === 'scanning' || state.status === 'queued').length;
    const totalSkills = locations.reduce((sum, location) => sum + (location.skill_count || 0), 0);
    const findings: SkillScanFinding[] = [];
    const filteredFindings = findings.filter((finding) => {
        const matchesSeverity = findingSeverity === 'all' || finding.severity === findingSeverity;
        const query = findingSearch.toLowerCase();
        const matchesSearch =
            query === '' ||
            finding.skillName.toLowerCase().includes(query) ||
            finding.tag.toLowerCase().includes(query) ||
            finding.filePath.toLowerCase().includes(query) ||
            finding.snippet.toLowerCase().includes(query);
        return matchesSeverity && matchesSearch;
    });
    const selectedFinding = filteredFindings.find((finding) => finding.id === selectedFindingId) || null;

    const findingSeverityMeta = {
        critical: { label: 'Critical', color: 'error' as const },
        high: { label: 'High', color: 'warning' as const },
        medium: { label: 'Medium', color: 'info' as const },
        low: { label: 'Low', color: 'default' as const },
    };

    return (
        <PageLayout loading={loading} notification={notification}>
            {/* Header */}
            <Box sx={{ mb: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Box>
                    <Typography variant="h4" sx={{ fontWeight: 600, mb: 0.5 }}>
                        Skill Scan
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Scan and review local AI skill locations from various IDEs and tools
                    </Typography>
                </Box>
                <Stack direction="row" spacing={1}>
                    <Button
                        variant="contained"
                        color="warning"
                        startIcon={scanRun.active ? <CircularProgress size={14} color="inherit" /> : <Refresh />}
                        onClick={handleScanAll}
                        size="small"
                        disabled={scanRun.active || locations.length === 0}
                    >
                        {scanRun.active ? 'Scanning…' : 'Scan All'}
                    </Button>
                    <Button
                        variant="outlined"
                        startIcon={<AutoFixHigh />}
                        onClick={() => setDiscoveryDialogOpen(true)}
                        size="small"
                    >
                        Auto Discover
                    </Button>
                    <Button
                        variant="contained"
                        startIcon={<Add />}
                        onClick={handleAddClick}
                        size="small"
                    >
                        Add Location
                    </Button>
                </Stack>
            </Box>

            <Paper sx={{ mb: 2, borderRadius: 2, overflow: 'hidden' }}>
                <Tabs
                    value={activeTab}
                    onChange={(_, value) => setActiveTab(value as SkillScanTab)}
                    variant="scrollable"
                    scrollButtons="auto"
                >
                    <Tab value="overview" label="Overview" />
                    <Tab value="skills" label={`Skills (${totalSkills})`} />
                    <Tab value="findings" label={`Findings (${findings.length})`} />
                </Tabs>
            </Paper>

            {activeTab === 'overview' && (
                <Stack spacing={2}>
                    <Paper
                        sx={{
                            p: 2,
                            border: 1,
                            borderColor: scanRun.active ? 'warning.light' : 'divider',
                            borderRadius: 2,
                            background: scanRun.active
                                ? 'linear-gradient(135deg, rgba(245,158,11,0.10) 0%, rgba(255,255,255,0.96) 100%)'
                                : 'background.paper',
                        }}
                    >
                        <Stack spacing={1.5}>
                            <Stack direction="row" justifyContent="space-between" alignItems="center" flexWrap="wrap" gap={1}>
                                <Box>
                                    <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>
                                        {scanRun.active ? 'Scanning local skill sources' : 'Scan workspace'}
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary">
                                        {scanRun.active
                                            ? `Processing ${scanRun.completed + 1} of ${scanRun.total} sources${currentLocationName ? ` · ${currentLocationName}` : ''}`
                                            : scanRun.finishedAt
                                                ? `Last scan completed at ${formatRelativeTime(scanRun.finishedAt)}`
                                                : 'Run a scan to refresh local skill metadata and review source health.'}
                                    </Typography>
                                </Box>
                                <Stack direction="row" spacing={1} flexWrap="wrap">
                                    <MuiChip size="small" label={`Sources ${locations.length}`} variant="outlined" />
                                    <MuiChip size="small" label={`Skills ${totalSkills}`} variant="outlined" />
                                    <MuiChip size="small" label={`Findings ${findings.length}`} variant="outlined" />
                                    <MuiChip size="small" label={`Failed ${failedSources}`} color="error" variant={failedSources > 0 ? 'filled' : 'outlined'} />
                                </Stack>
                            </Stack>
                            <LinearProgress
                                variant="determinate"
                                value={scanRun.active ? globalProgress : completedSources > 0 || failedSources > 0 ? 100 : 0}
                                sx={{ height: 10, borderRadius: 999 }}
                            />
                        </Stack>
                    </Paper>

                    {locations.length === 0 ? (
                        <UnifiedCard
                            title="No Skill Locations"
                            subtitle="Get started by discovering or adding your first local skill location"
                            size="large"
                        >
                            <Box textAlign="center" py={3}>
                                <Alert severity="info" sx={{ mb: 2, display: 'inline-block', textAlign: 'left' }}>
                                    <Typography variant="body2">
                                        <strong>About Skill Scan</strong><br />
                                        Skills are reusable AI prompts stored as markdown files in your IDE
                                        configuration directories. Tingly Box can discover, inspect, and
                                        review these local skills from multiple sources.
                                    </Typography>
                                </Alert>
                                <Stack direction="row" spacing={2} justifyContent="center" sx={{ mt: 2 }}>
                                    <Button variant="outlined" onClick={() => setDiscoveryDialogOpen(true)}>
                                        Auto Discover
                                    </Button>
                                    <Button variant="contained" onClick={handleAddClick}>
                                        Add Location Manually
                                    </Button>
                                </Stack>
                            </Box>
                        </UnifiedCard>
                    ) : (
                        <>
                            <Box
                                sx={{
                                    display: 'grid',
                                    gridTemplateColumns: { xs: '1fr', md: 'repeat(4, minmax(0, 1fr))' },
                                    gap: 1.5,
                                }}
                            >
                                {[
                                    { label: 'Sources', value: locations.length, tone: 'default' as const },
                                    { label: 'Scanned', value: completedSources, tone: 'success' as const },
                                    { label: 'Active', value: activeSources, tone: 'warning' as const },
                                    { label: 'Failed', value: failedSources, tone: 'error' as const },
                                ].map((card) => (
                                    <Paper key={card.label} sx={{ p: 2, border: 1, borderColor: 'divider', borderRadius: 2 }}>
                                        <Stack spacing={0.5}>
                                            <Typography variant="caption" color="text.secondary">
                                                {card.label}
                                            </Typography>
                                            <Typography variant="h5" sx={{ fontWeight: 700 }}>
                                                {card.value}
                                            </Typography>
                                            <MuiChip
                                                size="small"
                                                label={card.tone === 'default' ? 'Tracked' : card.label}
                                                color={card.tone}
                                                variant={card.value > 0 && card.tone !== 'default' ? 'filled' : 'outlined'}
                                                sx={{ alignSelf: 'flex-start' }}
                                            />
                                        </Stack>
                                    </Paper>
                                ))}
                            </Box>

                            <Paper sx={{ border: 1, borderColor: 'divider', borderRadius: 2, overflow: 'hidden' }}>
                                <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                                    <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                        Scan Sources
                                    </Typography>
                                    <Typography variant="body2" color="text.secondary">
                                        Source-by-source scan state and recent status.
                                    </Typography>
                                </Box>
                                <Stack spacing={1.5} sx={{ p: 2 }}>
                                    {filteredLocations.map((location) => {
                                        const state = sourceScanStates[location.id];
                                        const statusMeta = getSourceStatusMeta(state?.status || 'idle');
                                        return (
                                            <Paper
                                                key={location.id}
                                                variant="outlined"
                                                sx={{
                                                    p: 2,
                                                    borderRadius: 2,
                                                    cursor: 'pointer',
                                                    borderColor: selectedLocation?.id === location.id ? 'primary.main' : 'divider',
                                                }}
                                                onClick={() => {
                                                    setSelectedLocation(location);
                                                    setActiveTab('skills');
                                                }}
                                            >
                                                <Stack spacing={1}>
                                                    <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1}>
                                                        <Box sx={{ minWidth: 0 }}>
                                                            <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                                                {location.name}
                                                            </Typography>
                                                            <Typography variant="caption" color="text.secondary">
                                                                {getIdeSourceLabel(location.ide_source)}
                                                            </Typography>
                                                        </Box>
                                                        <MuiChip
                                                            label={statusMeta.label}
                                                            size="small"
                                                            color={statusMeta.color}
                                                            variant={state?.status === 'done' ? 'filled' : 'outlined'}
                                                        />
                                                    </Stack>
                                                    <Typography variant="body2" color="text.secondary">
                                                        {state?.status === 'done'
                                                            ? `Last run ${formatRelativeTime(state.lastScannedAt || location.last_scanned_at || scanRun.finishedAt)}`
                                                            : state?.status === 'failed'
                                                                ? state.error || 'Scan failed'
                                                                : scanRun.currentLocationId === location.id && scanRun.active
                                                                    ? 'Scanning source…'
                                                                    : 'Ready to scan'}
                                                    </Typography>
                                                    <LinearProgress
                                                        variant="determinate"
                                                        value={state?.progress || 0}
                                                        color={
                                                            state?.status === 'failed'
                                                                ? 'error'
                                                                : state?.status === 'done'
                                                                    ? 'success'
                                                                    : 'warning'
                                                        }
                                                        sx={{ height: 6, borderRadius: 999 }}
                                                    />
                                                </Stack>
                                            </Paper>
                                        );
                                    })}
                                </Stack>
                            </Paper>
                        </>
                    )}
                </Stack>
            )}

            {activeTab === 'skills' && (
                locations.length === 0 ? (
                    <UnifiedCard
                        title="No Skill Locations"
                        subtitle="Add a source before browsing local skills"
                        size="large"
                    >
                        <Box textAlign="center" py={3}>
                            <Stack direction="row" spacing={2} justifyContent="center">
                                <Button variant="outlined" onClick={() => setDiscoveryDialogOpen(true)}>
                                    Auto Discover
                                </Button>
                                <Button variant="contained" onClick={handleAddClick}>
                                    Add Location
                                </Button>
                            </Stack>
                        </Box>
                    </UnifiedCard>
                ) : (
                    <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 240px)' }}>
                        <Paper
                            sx={{
                                width: 300,
                                display: 'flex',
                                flexDirection: 'column',
                                border: 1,
                                borderColor: 'divider',
                                borderRadius: 2,
                                overflow: 'hidden',
                            }}
                        >
                            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                                    Scan Sources ({locations.length})
                                </Typography>
                                <TextField
                                    placeholder="Search..."
                                    value={locationSearch}
                                    onChange={(e) => setLocationSearch(e.target.value)}
                                    size="small"
                                    fullWidth
                                    InputProps={{
                                        startAdornment: (
                                            <InputAdornment position="start">
                                                <Search fontSize="small" />
                                            </InputAdornment>
                                        ),
                                    }}
                                />
                            </Box>
                            <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                                {filteredLocations.map((location) => {
                                    const isSelected = selectedLocation?.id === location.id;
                                    const state = sourceScanStates[location.id];
                                    const statusMeta = getSourceStatusMeta(state?.status || 'idle');
                                    return (
                                        <ListItem
                                            key={location.id}
                                            disablePadding
                                            divider
                                            sx={{ bgcolor: isSelected ? 'primary.50' : 'transparent' }}
                                        >
                                            <ListItemButton
                                                onClick={() => setSelectedLocation(location)}
                                                dense
                                                sx={{ py: 1.5, alignItems: 'flex-start' }}
                                            >
                                                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                                                        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                                            {location.name}
                                                        </Typography>
                                                        <MuiChip
                                                            label={statusMeta.label}
                                                            size="small"
                                                            color={statusMeta.color}
                                                            variant={state?.status === 'done' ? 'filled' : 'outlined'}
                                                            sx={{ height: 20, fontSize: '0.68rem' }}
                                                        />
                                                    </Box>
                                                    <MuiChip
                                                        label={getIdeSourceLabel(location.ide_source)}
                                                        size="small"
                                                        variant="outlined"
                                                        sx={{ alignSelf: 'flex-start', height: 20, fontSize: '0.7rem' }}
                                                    />
                                                    <Typography variant="caption" color="text.secondary">
                                                        {state?.status === 'done'
                                                            ? `Last run ${formatRelativeTime(state.lastScannedAt || location.last_scanned_at || scanRun.finishedAt)}`
                                                            : state?.status === 'failed'
                                                                ? state.error || 'Scan failed'
                                                                : scanRun.currentLocationId === location.id && scanRun.active
                                                                    ? 'Scanning source…'
                                                                    : 'Ready to scan'}
                                                    </Typography>
                                                    {(state?.status === 'scanning' || state?.status === 'done' || state?.status === 'failed') && (
                                                        <LinearProgress
                                                            variant="determinate"
                                                            value={state?.progress || 0}
                                                            color={
                                                                state?.status === 'failed'
                                                                    ? 'error'
                                                                    : state?.status === 'done'
                                                                        ? 'success'
                                                                        : 'warning'
                                                            }
                                                            sx={{ mt: 0.5, height: 6, borderRadius: 999 }}
                                                        />
                                                    )}
                                                </Box>
                                            </ListItemButton>
                                        </ListItem>
                                    );
                                })}
                            </List>
                        </Paper>

                        <Paper
                            sx={{
                                width: 320,
                                display: 'flex',
                                flexDirection: 'column',
                                border: 1,
                                borderColor: 'divider',
                                borderRadius: 2,
                                overflow: 'hidden',
                            }}
                        >
                            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                                    {selectedLocation ? selectedLocation.name : 'Skills'}
                                    {selectedLocation && ` (${skills.length})`}
                                </Typography>
                                <TextField
                                    placeholder="Search skills..."
                                    value={skillSearch}
                                    onChange={(e) => setSkillSearch(e.target.value)}
                                    size="small"
                                    fullWidth
                                    disabled={!selectedLocation}
                                    InputProps={{
                                        startAdornment: (
                                            <InputAdornment position="start">
                                                <Search fontSize="small" />
                                            </InputAdornment>
                                        ),
                                    }}
                                />
                            </Box>
                            <Box sx={{ flex: 1, overflow: 'auto' }}>
                                {!selectedLocation ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <FolderOpen sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
                                        <Typography variant="body2" color="text.secondary">
                                            Select a location to view skills
                                        </Typography>
                                    </Box>
                                ) : skillsLoading ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                        }}
                                    >
                                        <CircularProgress size={32} />
                                    </Box>
                                ) : filteredSkills.length === 0 ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <Description sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
                                        <Typography variant="body2" color="text.secondary">
                                            {skillSearch ? 'No skills match your search' : 'No skills found in this location'}
                                        </Typography>
                                    </Box>
                                ) : (
                                    <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                                        {filteredSkills.map((skill) => {
                                            const isSelected = selectedSkill?.id === skill.id;
                                            return (
                                                <ListItem
                                                    key={skill.id}
                                                    disablePadding
                                                    divider
                                                    sx={{ bgcolor: isSelected ? 'action.selected' : 'transparent' }}
                                                >
                                                    <ListItemButton
                                                        onClick={() => setSelectedSkill(skill)}
                                                        dense
                                                        sx={{ py: 1 }}
                                                    >
                                                        <Description
                                                            fontSize="small"
                                                            sx={{ mr: 1.5, color: 'action.active' }}
                                                        />
                                                        <ListItemText
                                                            primary={
                                                                <Typography variant="subtitle2" sx={{ fontWeight: 500 }}>
                                                                    {skill.name}
                                                                </Typography>
                                                            }
                                                        />
                                                    </ListItemButton>
                                                </ListItem>
                                            );
                                        })}
                                    </List>
                                )}
                            </Box>
                        </Paper>

                        <Paper
                            sx={{
                                flex: 1,
                                display: 'flex',
                                flexDirection: 'column',
                                border: 1,
                                borderColor: 'divider',
                                borderRadius: 2,
                                overflow: 'hidden',
                            }}
                        >
                            <Box
                                sx={{
                                    p: 2,
                                    borderBottom: 1,
                                    borderColor: 'divider',
                                    display: 'flex',
                                    justifyContent: 'space-between',
                                    alignItems: 'flex-start',
                                }}
                            >
                                <Box sx={{ minWidth: 0, flex: 1 }}>
                                    <Typography
                                        variant="subtitle1"
                                        sx={{
                                            fontWeight: 600,
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            whiteSpace: 'nowrap',
                                        }}
                                    >
                                        {selectedSkill ? selectedSkill.name : 'Skill Details'}
                                    </Typography>
                                </Box>
                                <Stack direction="row" spacing={0.5} alignItems="center">
                                    {skillContent && (
                                        <>
                                            <Button
                                                size="small"
                                                variant={viewMode === 'markdown' ? 'contained' : 'outlined'}
                                                startIcon={<Visibility />}
                                                onClick={() => setViewMode('markdown')}
                                                sx={{ minWidth: 32, px: 1 }}
                                            >
                                                Markdown
                                            </Button>
                                            <Button
                                                size="small"
                                                variant={viewMode === 'raw' ? 'contained' : 'outlined'}
                                                startIcon={<Code />}
                                                onClick={() => setViewMode('raw')}
                                                sx={{ minWidth: 32, px: 1 }}
                                            >
                                                Raw
                                            </Button>
                                            <IconButton
                                                size="small"
                                                onClick={handleCopyContent}
                                                disabled={contentLoading}
                                                title="Copy content"
                                            >
                                                <ContentCopy fontSize="small" />
                                            </IconButton>
                                        </>
                                    )}
                                </Stack>
                            </Box>
                            <Box sx={{ flex: 1, overflow: 'auto', bgcolor: 'background.default' }}>
                                {!selectedSkill ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <Description sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
                                        <Typography variant="body2" color="text.secondary">
                                            Select a skill to view its content
                                        </Typography>
                                    </Box>
                                ) : contentLoading ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                        }}
                                    >
                                        <CircularProgress size={32} />
                                    </Box>
                                ) : skillContent ? (
                                    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                                        {viewMode === 'markdown' ? (
                                            <Box
                                                sx={{
                                                    flex: 1,
                                                    overflow: 'auto',
                                                    '& .ant-md': {
                                                        bgcolor: 'background.paper',
                                                        p: 2,
                                                        minHeight: '100%',
                                                    },
                                                    '& .ant-markdown': {
                                                        fontSize: '0.875rem',
                                                        lineHeight: 1.6,
                                                    },
                                                }}
                                            >
                                                <XMarkdown style={{ height: '100%' }}>
                                                    {skillContent}
                                                </XMarkdown>
                                            </Box>
                                        ) : (
                                            <Box
                                                sx={{
                                                    p: 2,
                                                    fontFamily: 'monospace',
                                                    fontSize: '0.875rem',
                                                    whiteSpace: 'pre-wrap',
                                                    wordBreak: 'break-word',
                                                    lineHeight: 1.6,
                                                    flex: 1,
                                                    overflow: 'auto',
                                                }}
                                            >
                                                {skillContent}
                                            </Box>
                                        )}
                                    </Box>
                                ) : (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <Alert severity="info">
                                            <Typography variant="body2">
                                                No content available for this skill
                                            </Typography>
                                        </Alert>
                                    </Box>
                                )}
                            </Box>
                        </Paper>
                    </Stack>
                )
            )}

            {activeTab === 'findings' && (
                locations.length === 0 ? (
                    <UnifiedCard
                        title="No Findings Yet"
                        subtitle="Add and scan local skill sources before triaging findings"
                        size="large"
                    >
                        <Box textAlign="center" py={3}>
                            <Stack direction="row" spacing={2} justifyContent="center">
                                <Button variant="outlined" onClick={() => setDiscoveryDialogOpen(true)}>
                                    Auto Discover
                                </Button>
                                <Button variant="contained" onClick={handleAddClick}>
                                    Add Location
                                </Button>
                            </Stack>
                        </Box>
                    </UnifiedCard>
                ) : (
                    <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 240px)' }}>
                        <Paper
                            sx={{
                                width: 380,
                                display: 'flex',
                                flexDirection: 'column',
                                border: 1,
                                borderColor: 'divider',
                                borderRadius: 2,
                                overflow: 'hidden',
                            }}
                        >
                            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                                    Findings
                                </Typography>
                                <Stack spacing={1}>
                                    <TextField
                                        placeholder="Search findings..."
                                        value={findingSearch}
                                        onChange={(e) => setFindingSearch(e.target.value)}
                                        size="small"
                                        fullWidth
                                        InputProps={{
                                            startAdornment: (
                                                <InputAdornment position="start">
                                                    <Search fontSize="small" />
                                                </InputAdornment>
                                            ),
                                        }}
                                    />
                                    <Stack direction="row" spacing={1} flexWrap="wrap">
                                        <MuiChip
                                            size="small"
                                            label="All"
                                            clickable
                                            color={findingSeverity === 'all' ? 'primary' : 'default'}
                                            variant={findingSeverity === 'all' ? 'filled' : 'outlined'}
                                            onClick={() => setFindingSeverity('all')}
                                        />
                                        {(Object.keys(findingSeverityMeta) as FindingSeverity[]).map((severity) => (
                                            <MuiChip
                                                key={severity}
                                                size="small"
                                                label={findingSeverityMeta[severity].label}
                                                clickable
                                                color={findingSeverity === severity ? findingSeverityMeta[severity].color : 'default'}
                                                variant={findingSeverity === severity ? 'filled' : 'outlined'}
                                                onClick={() => setFindingSeverity(severity)}
                                            />
                                        ))}
                                    </Stack>
                                </Stack>
                            </Box>
                            <Box sx={{ flex: 1, overflow: 'auto' }}>
                                {filteredFindings.length === 0 ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <Alert severity="info">
                                            <Typography variant="body2">
                                                Findings will appear here after the backend scanner starts returning rule hits and line-level detail.
                                            </Typography>
                                        </Alert>
                                    </Box>
                                ) : (
                                    <List sx={{ p: 0 }}>
                                        {filteredFindings.map((finding) => (
                                            <ListItem key={finding.id} disablePadding divider>
                                                <ListItemButton onClick={() => setSelectedFindingId(finding.id)} dense>
                                                    <ListItemText
                                                        primary={
                                                            <Stack direction="row" spacing={1} alignItems="center">
                                                                <MuiChip
                                                                    size="small"
                                                                    label={findingSeverityMeta[finding.severity].label}
                                                                    color={findingSeverityMeta[finding.severity].color}
                                                                    variant="filled"
                                                                />
                                                                <Typography variant="subtitle2">{finding.skillName}</Typography>
                                                            </Stack>
                                                        }
                                                        secondary={
                                                            <Typography variant="caption" color="text.secondary">
                                                                {finding.tag} · {finding.filePath}:{finding.line}
                                                            </Typography>
                                                        }
                                                    />
                                                </ListItemButton>
                                            </ListItem>
                                        ))}
                                    </List>
                                )}
                            </Box>
                        </Paper>

                        <Paper
                            sx={{
                                flex: 1,
                                display: 'flex',
                                flexDirection: 'column',
                                border: 1,
                                borderColor: 'divider',
                                borderRadius: 2,
                                overflow: 'hidden',
                            }}
                        >
                            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    Finding Detail
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Inspect matched tags, file paths, snippets, and the exact affected skill.
                                </Typography>
                            </Box>
                            <Box sx={{ flex: 1, overflow: 'auto', bgcolor: 'background.default' }}>
                                {!selectedFinding ? (
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            height: '100%',
                                            p: 3,
                                            textAlign: 'center',
                                        }}
                                    >
                                        <Description sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }} />
                                        <Typography variant="body2" color="text.secondary">
                                            Select a finding to inspect the affected file, line, and snippet.
                                        </Typography>
                                    </Box>
                                ) : (
                                    <Stack spacing={2} sx={{ p: 3 }}>
                                        <Stack direction="row" spacing={1} alignItems="center">
                                            <MuiChip
                                                size="small"
                                                label={findingSeverityMeta[selectedFinding.severity].label}
                                                color={findingSeverityMeta[selectedFinding.severity].color}
                                                variant="filled"
                                            />
                                            <MuiChip size="small" label={selectedFinding.tag} variant="outlined" />
                                        </Stack>
                                        <Box>
                                            <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                                {selectedFinding.skillName}
                                            </Typography>
                                            <Typography variant="body2" color="text.secondary">
                                                {selectedFinding.sourceName}
                                            </Typography>
                                        </Box>
                                        <Paper variant="outlined" sx={{ p: 2 }}>
                                            <Typography variant="caption" color="text.secondary">
                                                {selectedFinding.filePath}:{selectedFinding.line}
                                            </Typography>
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    mt: 1,
                                                    fontFamily: 'monospace',
                                                    whiteSpace: 'pre-wrap',
                                                    wordBreak: 'break-word',
                                                }}
                                            >
                                                {selectedFinding.snippet}
                                            </Typography>
                                        </Paper>
                                    </Stack>
                                )}
                            </Box>
                        </Paper>
                    </Stack>
                )
            )}

            {/* Add Location Dialog */}
            <AddSkillLocationDialog
                open={addDialogOpen}
                onClose={() => setAddDialogOpen(false)}
                onSubmit={handleAddSubmit}
            />

            {/* Auto Discovery Dialog */}
            <AutoDiscoveryDialog
                open={discoveryDialogOpen}
                onClose={() => setDiscoveryDialogOpen(false)}
                onImport={handleImportLocations}
            />
        </PageLayout>
    );
};

export default SkillScanPage;
