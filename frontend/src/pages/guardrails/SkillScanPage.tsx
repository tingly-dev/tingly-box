import {
    Add,
    AutoFixHigh,
    Code,
    ContentCopy,
    Description,
    ExpandLess,
    ExpandMore,
    FolderOpen,
    Refresh,
    Search,
    Visibility,
    ViewList,
} from '@mui/icons-material';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Collapse,
    Divider,
    IconButton,
    InputAdornment,
    LinearProgress,
    List,
    ListItem,
    ListItemButton,
    ListItemText,
    Paper,
    Stack,
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

const normalizePathLike = (value: string): string => {
    if (!value) return '';
    return value
        .replace(/\\/g, '/')
        .replace(/\/+/g, '/')
        .replace(/(^|\/)\.(?=\/|$)/g, '$1');
};

const splitPathSegments = (value: string): string[] => {
    const normalized = normalizePathLike(value);
    if (normalized === '') return [];
    return normalized.split('/').filter(part => part !== '' && part !== '.');
};

const normalizePatternForMatch = (value: string): string => {
    return splitPathSegments(value).join('/');
};

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
    const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
    const [isGroupedMode, setIsGroupedMode] = useState(true);

    // Skill detail state
    const [skillContent, setSkillContent] = useState<string>('');
    const [contentLoading, setContentLoading] = useState(false);
    const [viewMode, setViewMode] = useState<'markdown' | 'raw'>('raw');

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
            // Reset expanded groups for new location, but auto-expand first group
            setExpandedGroups(new Set());
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
            skill.path
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

    const handleCopyPath = () => {
        if (selectedSkill) {
            navigator.clipboard.writeText(selectedSkill.path);
            showNotification('Path copied to clipboard!', 'success');
        }
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
            skill.filename.toLowerCase().includes(skillSearch.toLowerCase());
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

    const getRelativePath = (skill: Skill, location: SkillLocation): string => {
        const basePath = location.path.endsWith('/') ? location.path : location.path + '/';
        if (skill.path.startsWith(basePath)) {
            return skill.path.substring(basePath.length);
        }
        return skill.filename;
    };

    const getSkillDisplayName = (skill: Skill, location: SkillLocation): string => {
        const relativePath = getRelativePath(skill, location);
        const parts = splitPathSegments(relativePath);
        // If file is in a subdirectory, include parent directory
        if (parts.length > 1) {
            const parentDir = parts[parts.length - 2];
            const fileName = parts[parts.length - 1];
            return `${parentDir}/${fileName}`;
        }
        // Otherwise just use the filename
        return relativePath;
    };

    // Get a two-level display name (last two levels) for flat mode
    const getTwoLevelDisplayName = (skill: Skill, location: SkillLocation): string => {
        const relativePath = getRelativePath(skill, location);
        const parts = splitPathSegments(relativePath);

        // Get last two levels: file and its parent
        if (parts.length >= 2) {
            const parentDir = parts[parts.length - 2];
            const fileName = parts[parts.length - 1];
            return `${parentDir}/${fileName}`;
        }
        // Single level
        return relativePath;
    };

    // Helper: Find prefix in path that contains the pattern
    // Pattern examples:
    // - "/skills/" -> find "/skills/" in path, prefix is everything up to and including the match
    // - "skills" -> find "skills" in path, prefix is everything up to and including the match
    const getGroupKeyFromPattern = (pattern: string, pathParts: string[]): { groupKey: string; matched: boolean } => {
        // Build path string and find pattern
        const pathStr = pathParts.join('/');
        const normalizedPattern = normalizePatternForMatch(pattern);
        if (normalizedPattern === '') {
            return { groupKey: '', matched: false };
        }
        const patternIndex = pathStr.indexOf(normalizedPattern);

        if (patternIndex === -1) {
            return { groupKey: '', matched: false };
        }

        // Extract prefix: everything before and including the matched pattern
        const matchEnd = patternIndex + normalizedPattern.length;
        const prefix = pathStr.substring(0, matchEnd);

        // Remove trailing slash if present (except for root)
        const groupKey = prefix.endsWith('/') && prefix.length > 1 ? prefix.slice(0, -1) : prefix;

        return { groupKey, matched: true };
    };

    // Group skills based on location's grouping strategy
    const groupSkillsIntelligently = (skills: Skill[], location: SkillLocation | null): Array<{ groupKey: string; groupLabel: string; skills: Skill[]; level: number }> => {
        if (!location) return [{ groupKey: '', groupLabel: '(root)', skills, level: 0 }];

        // Get grouping strategy from location, default to auto mode
        const strategy = location.grouping_strategy || { mode: 'auto' as const, min_files_for_split: 5 };
        const mode = strategy.mode || 'auto';
        const minFilesForSplit = strategy.min_files_for_split || 5;

        const result: Array<{ groupKey: string; groupLabel: string; skills: Skill[]; level: number }> = [];

        // FLAT MODE: No grouping, just list all files
        if (mode === 'flat') {
            return [{ groupKey: '', groupLabel: 'All Skills', skills, level: 0 }];
        }

        // PATTERN MODE: Group by finding pattern in path
        // Pattern examples:
        // - "/skills/" -> any path containing "/skills/" gets grouped to "skills"
        // - "skills" -> any path containing "skills" gets grouped to "skills" (or "xxx/skills")
        if (mode === 'pattern' && strategy.group_pattern) {
            const pattern = strategy.group_pattern;

            // Group files by matching the pattern
            const patternGroups: Record<string, Skill[]> = {};
            const otherFiles: Skill[] = [];

            for (const skill of skills) {
                const relativePath = getRelativePath(skill, location);
                const parts = splitPathSegments(relativePath);

                const { groupKey, matched } = getGroupKeyFromPattern(pattern, parts);

                if (matched && groupKey) {
                    if (!patternGroups[groupKey]) {
                        patternGroups[groupKey] = [];
                    }
                    patternGroups[groupKey].push(skill);
                } else {
                    otherFiles.push(skill);
                }
            }

            // Add pattern-matched groups
            for (const [groupKey, groupSkills] of Object.entries(patternGroups)) {
                // Split further if too many files
                if (groupSkills.length > minFilesForSplit && shouldSplitIntoSubGroups(groupSkills, location)) {
                    const subGroups = splitIntoSubGroups(groupSkills, location, groupKey);
                    result.push(...subGroups);
                } else {
                    result.push({
                        groupKey,
                        groupLabel: groupKey,
                        skills: groupSkills,
                        level: 1,
                    });
                }
            }

            // Add other files
            if (otherFiles.length > 0) {
                result.push({
                    groupKey: '',
                    groupLabel: '(other)',
                    skills: otherFiles,
                    level: 0,
                });
            }

            // Sort groups
            result.sort((a, b) => {
                if (a.groupKey === '') return 1;
                if (b.groupKey === '') return -1;
                return a.groupKey.localeCompare(b.groupKey);
            });

            return result;
        }

        // AUTO MODE: Automatic grouping based on file count and structure
        const firstLevelGroups: Record<string, Skill[]> = {};
        const rootFiles: Skill[] = [];

        for (const skill of skills) {
            const relativePath = getRelativePath(skill, location);
            const parts = splitPathSegments(relativePath);

            if (parts.length === 1) {
                rootFiles.push(skill);
            } else {
                const firstLevelDir = parts[0];
                if (!firstLevelGroups[firstLevelDir]) {
                    firstLevelGroups[firstLevelDir] = [];
                }
                firstLevelGroups[firstLevelDir].push(skill);
            }
        }

        // Add root files group
        if (rootFiles.length > 0) {
            result.push({
                groupKey: '',
                groupLabel: '(root)',
                skills: rootFiles,
                level: 0,
            });
        }

        // Process each first-level directory
        for (const [dirName, dirSkills] of Object.entries(firstLevelGroups)) {
            if (dirSkills.length > minFilesForSplit && shouldSplitIntoSubGroups(dirSkills, location)) {
                const subGroups = splitIntoSubGroups(dirSkills, location, dirName);
                result.push(...subGroups);
            } else {
                result.push({
                    groupKey: dirName,
                    groupLabel: dirName,
                    skills: dirSkills,
                    level: 1,
                });
            }
        }

        // Sort groups
        result.sort((a, b) => {
            if (a.level !== b.level) return a.level - b.level;
            if (a.groupKey === '') return 1;
            if (b.groupKey === '') return -1;
            return a.groupKey.localeCompare(b.groupKey);
        });

        return result;
    };

    // Helper: Check if a group should be split into sub-groups
    const shouldSplitIntoSubGroups = (groupSkills: Skill[], location: SkillLocation): boolean => {
        const subGroups: Record<string, Skill[]> = {};
        for (const skill of groupSkills) {
            const relativePath = getRelativePath(skill, location);
            const parts = splitPathSegments(relativePath);
            if (parts.length >= 2) {
                const secondLevelDir = parts[1];
                if (!subGroups[secondLevelDir]) {
                    subGroups[secondLevelDir] = [];
                }
                subGroups[secondLevelDir].push(skill);
            }
        }
        return Object.keys(subGroups).length >= 2;
    };

    // Helper: Split a group into sub-groups based on second-level directory
    const splitIntoSubGroups = (groupSkills: Skill[], location: SkillLocation, parentDir: string): Array<{ groupKey: string; groupLabel: string; skills: Skill[]; level: number }> => {
        const subGroups: Record<string, Skill[]> = {};
        const rootFiles: Skill[] = [];

        for (const skill of groupSkills) {
            const relativePath = getRelativePath(skill, location);
            const parts = splitPathSegments(relativePath);

            if (parts.length >= 2) {
                const secondLevelDir = parts[1];
                const key = `${parentDir}/${secondLevelDir}`;
                if (!subGroups[key]) {
                    subGroups[key] = [];
                }
                subGroups[key].push(skill);
            } else {
                rootFiles.push(skill);
            }
        }

        const result: Array<{ groupKey: string; groupLabel: string; skills: Skill[]; level: number }> = [];

        // Add root files in this directory
        if (rootFiles.length > 0) {
            result.push({
                groupKey: parentDir,
                groupLabel: parentDir,
                skills: rootFiles,
                level: 1,
            });
        }

        // Add sub-groups
        for (const [subKey, subSkills] of Object.entries(subGroups)) {
            result.push({
                groupKey: subKey,
                groupLabel: subKey,
                skills: subSkills,
                level: 2,
            });
        }

        return result;
    };

    const toggleGroup = (groupKey: string) => {
        setExpandedGroups(prev => {
            const newSet = new Set(prev);
            if (newSet.has(groupKey)) {
                newSet.delete(groupKey);
            } else {
                newSet.add(groupKey);
            }
            return newSet;
        });
    };

    const isGroupExpanded = (groupKey: string) => {
        // Auto-expand if it's the only group or if search is active
        if (skillSearch !== '') return true;
        return expandedGroups.has(groupKey);
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

            {locations.length > 0 && (
                <Paper
                    sx={{
                        mb: 2,
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
                                <MuiChip size="small" label={`Scanned ${completedSources}`} color="success" variant={completedSources > 0 ? 'filled' : 'outlined'} />
                                <MuiChip size="small" label={`Active ${activeSources}`} color="warning" variant={activeSources > 0 ? 'filled' : 'outlined'} />
                                <MuiChip size="small" label={`Failed ${failedSources}`} color="error" variant={failedSources > 0 ? 'filled' : 'outlined'} />
                            </Stack>
                        </Stack>
                        <LinearProgress
                            variant={scanRun.active ? 'determinate' : 'determinate'}
                            value={scanRun.active ? globalProgress : completedSources > 0 || failedSources > 0 ? 100 : 0}
                            sx={{ height: 10, borderRadius: 999 }}
                        />
                    </Stack>
                </Paper>
            )}

            {/* Empty State */}
            {locations.length === 0 && !loading && (
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
                            <Button
                                variant="outlined"
                                onClick={() => setDiscoveryDialogOpen(true)}
                            >
                                Auto Discover
                            </Button>
                            <Button variant="contained" onClick={handleAddClick}>
                                Add Location Manually
                            </Button>
                        </Stack>
                    </Box>
                </UnifiedCard>
            )}

            {/* Three-Column Layout */}
            {locations.length > 0 && (
                <Stack direction="row" spacing={1} sx={{ height: 'calc(100vh - 180px)' }}>
                    {/* Column 1: Locations List */}
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
                                return (
                                    <ListItem
                                        key={location.id}
                                        disablePadding
                                        divider
                                        sx={{
                                            bgcolor: isSelected ? 'primary.50' : 'transparent',
                                        }}
                                    >
                                        <ListItemButton
                                            onClick={() => setSelectedLocation(location)}
                                            dense
                                            sx={{ py: 1.5, alignItems: 'flex-start' }}
                                        >
                                            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                                                    <Typography
                                                        variant="subtitle2"
                                                        sx={{ fontWeight: 600 }}
                                                    >
                                                        {location.name}
                                                    </Typography>
                                                    <MuiChip
                                                        label={getSourceStatusMeta(sourceScanStates[location.id]?.status || 'idle').label}
                                                        size="small"
                                                        color={getSourceStatusMeta(sourceScanStates[location.id]?.status || 'idle').color}
                                                        variant={sourceScanStates[location.id]?.status === 'done' ? 'filled' : 'outlined'}
                                                        sx={{ height: 20, fontSize: '0.68rem' }}
                                                    />
                                                </Box>
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
                                                <MuiChip
                                                    label={getIdeSourceLabel(location.ide_source)}
                                                    size="small"
                                                    variant="outlined"
                                                    sx={{ alignSelf: 'flex-start', height: 20, fontSize: '0.7rem' }}
                                                />
                                                    <Typography variant="caption" color="text.secondary">
                                                        {sourceScanStates[location.id]?.status === 'done'
                                                        ? `Last run ${formatRelativeTime(sourceScanStates[location.id]?.lastScannedAt || location.last_scanned_at || scanRun.finishedAt)}`
                                                        : sourceScanStates[location.id]?.status === 'failed'
                                                            ? sourceScanStates[location.id]?.error || 'Scan failed'
                                                            : scanRun.currentLocationId === location.id && scanRun.active
                                                                ? 'Scanning source…'
                                                                : 'Ready to scan'}
                                                </Typography>
                                                {(sourceScanStates[location.id]?.status === 'scanning' || sourceScanStates[location.id]?.status === 'done' || sourceScanStates[location.id]?.status === 'failed') && (
                                                    <LinearProgress
                                                        variant="determinate"
                                                        value={sourceScanStates[location.id]?.progress || 0}
                                                        color={
                                                            sourceScanStates[location.id]?.status === 'failed'
                                                                ? 'error'
                                                                : sourceScanStates[location.id]?.status === 'done'
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

                    {/* Column 2: Skills List */}
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
                            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                    {selectedLocation ? selectedLocation.name : 'Skills'}
                                    {selectedLocation && ` (${skills.length})`}
                                </Typography>
                                <IconButton
                                    size="small"
                                    onClick={() => setIsGroupedMode(!isGroupedMode)}
                                    disabled={!selectedLocation}
                                    title={isGroupedMode ? 'Switch to flat view' : 'Switch to grouped view'}
                                >
                                    {isGroupedMode ? <ViewList fontSize="small" /> : <Description fontSize="small" />}
                                </IconButton>
                            </Box>
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
                                    <FolderOpen
                                        sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }}
                                    />
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
                                    <Description
                                        sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }}
                                    />
                                    <Typography variant="body2" color="text.secondary">
                                        {skillSearch
                                            ? 'No skills match your search'
                                            : 'No skills found in this location'}
                                    </Typography>
                                </Box>
                            ) : (
                                <Box sx={{ flex: 1, overflow: 'auto' }}>
                                    {isGroupedMode ? (
                                        // Grouped mode
                                        (() => {
                                            const skillGroups = groupSkillsIntelligently(filteredSkills, selectedLocation);

                                            return skillGroups.map((group) => {
                                                const isExpanded = isGroupExpanded(group.groupKey);
                                                const groupLabel = group.groupLabel;

                                                return (
                                                    <Box key={group.groupKey}>
                                                        {/* Group Header */}
                                                        <ListItem
                                                            disablePadding
                                                            sx={{ borderBottom: 1, borderColor: 'divider' }}
                                                        >
                                                            <ListItemButton
                                                                onClick={() => toggleGroup(group.groupKey)}
                                                                dense
                                                                sx={{ py: 0.75, px: 2 }}
                                                            >
                                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                                                                    {isExpanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
                                                                    <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                                                        {groupLabel}
                                                                    </Typography>
                                                                    <MuiChip
                                                                        label={group.skills.length}
                                                                        size="small"
                                                                        sx={{ height: 18, fontSize: '0.65rem' }}
                                                                    />
                                                                </Box>
                                                            </ListItemButton>
                                                        </ListItem>

                                                        {/* Group Content */}
                                                        <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                                                            <List sx={{ p: 0 }}>
                                                                {group.skills.map((skill) => {
                                                                    const isSelected = selectedSkill?.id === skill.id;
                                                                    const relativePath = selectedLocation ? getRelativePath(skill, selectedLocation) : skill.filename;
                                                                    // Display path: remove group prefix if exists
                                                                    const displayPath = group.groupKey && relativePath.startsWith(group.groupKey + '/')
                                                                        ? relativePath.substring(group.groupKey.length + 1)
                                                                        : relativePath;
                                                                    // Get two-level display name
                                                                    const twoLevelName = getTwoLevelDisplayName(skill, selectedLocation || { path: '', ide_source: 'custom' as const, name: '' });
                                                                    return (
                                                                        <ListItem
                                                                            key={skill.id}
                                                                            disablePadding
                                                                            divider
                                                                            sx={{
                                                                                bgcolor: isSelected
                                                                                    ? 'action.selected'
                                                                                    : 'transparent',
                                                                                pl: 2,
                                                                            }}
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
                                                                                        <Typography
                                                                                            variant="subtitle2"
                                                                                            sx={{ fontWeight: 500 }}
                                                                                        >
                                                                                            {twoLevelName}
                                                                                        </Typography>
                                                                                    }
                                                                                    secondary={
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
                                                                                            {displayPath}
                                                                                        </Typography>
                                                                                    }
                                                                                />
                                                                            </ListItemButton>
                                                                        </ListItem>
                                                                    );
                                                                })}
                                                            </List>
                                                        </Collapse>
                                                    </Box>
                                                );
                                            });
                                        })()
                                    ) : (
                                        // Flat mode
                                        <List sx={{ p: 0 }}>
                                            {filteredSkills.map((skill) => {
                                                const isSelected = selectedSkill?.id === skill.id;
                                                const twoLevelName = selectedLocation ? getTwoLevelDisplayName(skill, selectedLocation) : skill.filename;
                                                const relativePath = selectedLocation ? getRelativePath(skill, selectedLocation) : skill.filename;
                                                return (
                                                    <ListItem
                                                        key={skill.id}
                                                        disablePadding
                                                        divider
                                                        sx={{
                                                            bgcolor: isSelected
                                                                ? 'action.selected'
                                                                : 'transparent',
                                                        }}
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
                                                                    <Typography
                                                                        variant="subtitle2"
                                                                        sx={{ fontWeight: 500 }}
                                                                    >
                                                                        {twoLevelName}
                                                                    </Typography>
                                                                }
                                                                secondary={
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
                                                                        {relativePath}
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
                            )}
                        </Box>
                    </Paper>

                    {/* Column 3: Skill Detail */}
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
                                    {selectedSkill && selectedLocation ? getTwoLevelDisplayName(selectedSkill, selectedLocation) : (selectedSkill ? selectedSkill.name : 'Skill Details')}
                                </Typography>
                                {selectedSkill && (
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
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
                                            {selectedSkill.path}
                                        </Typography>
                                        <IconButton
                                            size="small"
                                            onClick={handleCopyPath}
                                            sx={{ ml: -0.5 }}
                                            title="Copy path"
                                        >
                                            <ContentCopy fontSize="small" />
                                        </IconButton>
                                    </Box>
                                )}
                                {selectedSkill && (
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
                                        {formatFileSize(selectedSkill.size)}
                                    </Typography>
                                )}
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
                                    <Description
                                        sx={{ fontSize: 64, color: 'text.disabled', mb: 2 }}
                                    />
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
                                            <XMarkdown
                                                style={{
                                                    height: '100%',
                                                }}
                                            >
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
