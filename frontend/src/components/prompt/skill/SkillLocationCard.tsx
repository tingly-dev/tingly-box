import {
    Delete,
    Edit,
    FolderOpen,
    Refresh,
    Visibility,
} from '@mui/icons-material';
import {
    Box,
    Card,
    CardContent,
    Chip,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { type SkillLocation } from '@/types/prompt';
import { getIdeSourceIcon } from '@/constants/ideSources';

interface SkillLocationCardProps {
    location: SkillLocation;
    onRefresh: (id: string) => void;
    onEdit: (location: SkillLocation) => void;
    onDelete: (id: string) => void;
    onViewSkills: (location: SkillLocation) => void;
}

const SkillLocationCard = ({
    location,
    onRefresh,
    onEdit,
    onDelete,
    onViewSkills,
}: SkillLocationCardProps) => {
    const icon = location.icon || getIdeSourceIcon(location.ide_source);
    const lastScanned = location.last_scanned_at
        ? new Date(location.last_scanned_at).toLocaleString()
        : 'Never';

    return (
        <Card
            sx={{
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                border: 1,
                borderColor: 'divider',
                borderRadius: 2,
                transition: 'all 0.2s',
                '&:hover': {
                    boxShadow: 3,
                    transform: 'translateY(-2px)',
                },
            }}
        >
            <CardContent sx={{ flexGrow: 1, p: 2 }}>
                <Box
                    sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'flex-start',
                        mb: 2,
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
                        <Typography sx={{ fontSize: 24 }}>{icon}</Typography>
                        <Box sx={{ minWidth: 0, flex: 1 }}>
                            <Typography
                                variant="h6"
                                sx={{
                                    fontWeight: 600,
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
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
                </Box>

                <Stack direction="row" spacing={1} mb={2} flexWrap="wrap">
                    <Chip
                        size="small"
                        label={`${location.skill_count} skill${location.skill_count !== 1 ? 's' : ''}`}
                        color="primary"
                        variant="outlined"
                    />
                    {location.is_auto_discovered && (
                        <Chip size="small" label="Auto-discovered" variant="outlined" />
                    )}
                    {location.is_installed && (
                        <Chip size="small" label="Installed" color="success" variant="outlined" />
                    )}
                </Stack>

                <Typography variant="caption" color="text.secondary" display="block" mb={2}>
                    Last scanned: {lastScanned}
                </Typography>

                <Stack
                    direction="row"
                    spacing={0.5}
                    sx={{ justifyContent: 'flex-end' }}
                >
                    <Tooltip title="Refresh">
                        <IconButton
                            size="small"
                            color="primary"
                            onClick={() => onRefresh(location.id)}
                        >
                            <Refresh fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Edit">
                        <IconButton
                            size="small"
                            color="primary"
                            onClick={() => onEdit(location)}
                        >
                            <Edit fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                        <IconButton
                            size="small"
                            color="error"
                            onClick={() => onDelete(location.id)}
                        >
                            <Delete fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="View Skills">
                        <IconButton
                            size="small"
                            color="info"
                            onClick={() => onViewSkills(location)}
                        >
                            <Visibility fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Open Folder">
                        <IconButton
                            size="small"
                            color="success"
                            onClick={() => onViewSkills(location)}
                        >
                            <FolderOpen fontSize="small" />
                        </IconButton>
                    </Tooltip>
                </Stack>
            </CardContent>
        </Card>
    );
};

export default SkillLocationCard;
