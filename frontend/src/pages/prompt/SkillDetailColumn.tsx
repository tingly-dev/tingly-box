import { Code, ContentCopy, Description, Visibility } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import XMarkdown from '@ant-design/x-markdown';
import { type Skill, type SkillLocation } from '@/types/prompt';
import { formatFileSize, getTwoLevelDisplayName } from '@/components/prompt/skill/skillGrouping';
import SkillColumnShell from './SkillColumnShell';

interface SkillDetailColumnProps {
    selectedSkill: Skill | null;
    selectedLocation: SkillLocation | null;
    content: string;
    contentLoading: boolean;
    viewMode: 'markdown' | 'raw';
    onViewModeChange: (mode: 'markdown' | 'raw') => void;
    onCopyContent: () => void;
    onCopyPath: () => void;
}

const SkillDetailColumn = ({
    selectedSkill,
    selectedLocation,
    content,
    contentLoading,
    viewMode,
    onViewModeChange,
    onCopyContent,
    onCopyPath,
}: SkillDetailColumnProps) => (
    <SkillColumnShell
        flex={1}
        header={
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
                                sx={{
                                    color: "text.secondary",
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                    display: 'block'
                                }}>
                                {selectedSkill.path}
                            </Typography>
                            <IconButton
                                size="small"
                                onClick={onCopyPath}
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
                            sx={{
                                color: "text.secondary",
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                                display: 'block'
                            }}>
                            {formatFileSize(selectedSkill.size)}
                        </Typography>
                    )}
                </Box>
                <Stack direction="row" spacing={0.5} sx={{
                    alignItems: "center"
                }}>
                    {content && (
                        <>
                            <Button
                                size="small"
                                variant={viewMode === 'markdown' ? 'contained' : 'outlined'}
                                startIcon={<Visibility />}
                                onClick={() => onViewModeChange('markdown')}
                                sx={{ minWidth: 32, px: 1 }}
                            >
                                Markdown
                            </Button>
                            <Button
                                size="small"
                                variant={viewMode === 'raw' ? 'contained' : 'outlined'}
                                startIcon={<Code />}
                                onClick={() => onViewModeChange('raw')}
                                sx={{ minWidth: 32, px: 1 }}
                            >
                                Raw
                            </Button>
                            <IconButton
                                size="small"
                                onClick={onCopyContent}
                                disabled={contentLoading}
                                title="Copy content"
                            >
                                <ContentCopy fontSize="small" />
                            </IconButton>
                        </>
                    )}
                </Stack>
            </Box>
        }
    >
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
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
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
            ) : content ? (
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
                                {content}
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
                            {content}
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
    </SkillColumnShell>
);

export default SkillDetailColumn;
