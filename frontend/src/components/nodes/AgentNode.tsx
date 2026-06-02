import { Box, Chip, Divider, Paper, Popover, Typography, styled } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';
import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

type AgentType = 'claude-code' | 'smart-guide' | 'custom' | 'mock';

interface AgentInfo {
    description: string;
    features: string[];
    config: string;
}

type Lang = 'en' | 'zh';

// Bilingual content is intentionally co-located here rather than in the
// i18n locale files (zh.ts / en.ts). These strings are graph-node popover
// copy that is tightly coupled to the AgentNode component; externalising
// them would scatter context-specific copy across the global translation
// namespace without benefit. Do NOT migrate these strings to the locale
// files — use the Record<Lang, AgentInfo> pattern below to add translations.
const AGENT_TYPE_CONFIG: Record<AgentType, {
    label: string;
    color: 'info' | 'success' | 'default' | 'warning';
    info: Record<Lang, AgentInfo>;
}> = {
    'claude-code': {
        label: 'Claude Code',
        color: 'info',
        info: {
            en: {
                description: 'A full-spectrum development agent powered by Claude Code CLI. Handles implementation, refactoring, testing, builds, and git operations — anything that requires deep code execution in your local environment.',
                features: [
                    'Implement features, algorithms, and multi-file refactors',
                    'Run tests, builds, and debug sessions',
                    'Execute git operations: commits, pushes, rebases',
                    'Manage dependencies and complex package changes',
                    'Triggered via IM — type @cc to hand off from SmartGuide',
                ],
                config: 'Click this node to open the Claude Code scene and configure profiles.',
            },
            zh: {
                description: '由 Claude Code CLI 驱动的全栈开发代理，负责功能实现、重构、测试、构建和 Git 操作——所有需要在本地环境深度执行的任务。',
                features: [
                    '实现功能、算法及跨文件重构',
                    '运行测试、构建与调试',
                    '执行 Git 操作：提交、推送、变基',
                    '管理依赖和复杂包变更',
                    '通过 IM 触发——在 SmartGuide 中输入 @cc 即可切换',
                ],
                config: '点击此节点跳转到 Claude Code 场景页，配置 Profile 和运行参数。',
            },
        },
    },
    'smart-guide': {
        label: 'SmartGuide',
        color: 'success',
        info: {
            en: {
                description: 'A navigation and coordination assistant (@tb). Understands your intent, explores the project, answers questions, handles small edits and workspace setup — then hands off heavy implementation to @cc.',
                features: [
                    'Project exploration: read files, explain structure and architecture',
                    'Answer questions about dependencies and configuration',
                    'Small precise edits: config values, env vars, templates',
                    'Workspace setup: clone repos, set working directory',
                    'Persistent memory across sessions (MEMORY.md)',
                ],
                config: 'Click the Model node to the right to select the provider and model for this agent.',
            },
            zh: {
                description: '导航与协调助手（@tb）。理解用户意图、探索项目、回答问题、处理小改动和工作区配置，重度实现任务交由 @cc 处理。',
                features: [
                    '项目探索：读取文件，解释结构和架构',
                    '回答依赖与配置相关问题',
                    '精准小改动：配置值、环境变量、模板',
                    '工作区配置：克隆仓库、设置工作目录',
                    '跨会话持久记忆（MEMORY.md）',
                ],
                config: '点击右侧 Model 节点，为此代理选择服务商和模型。',
            },
        },
    },
    'custom': {
        label: 'Custom',
        color: 'warning',
        info: {
            en: {
                description: 'A custom agent implementation with user-defined behavior and endpoints.',
                features: ['User-defined request/response handling', 'Custom tool integrations'],
                config: 'Configure via the agent settings panel.',
            },
            zh: {
                description: '用户自定义行为和端点的自定义代理实现。',
                features: ['自定义请求/响应处理', '自定义工具集成'],
                config: '通过代理设置面板进行配置。',
            },
        },
    },
    'mock': {
        label: 'Mock',
        color: 'default',
        info: {
            en: {
                description: 'A mock agent for testing and development. Returns predefined responses without external API calls.',
                features: ['Predefined test responses', 'No external API calls', 'Useful for UI testing'],
                config: 'No configuration required.',
            },
            zh: {
                description: '用于测试和开发的 Mock 代理，返回预设响应，不发起外部 API 调用。',
                features: ['预设测试响应', '无外部 API 调用', '适合 UI 测试'],
                config: '无需任何配置。',
            },
        },
    },
};

const StyledAgentNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable',
})<{ active: boolean; clickable: boolean }>(({ active, clickable, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 12,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'primary.main' : 'divider',
    backgroundColor: active ? 'primary.50' : 'background.paper',
    textAlign: 'center',
    width: 220,
    height: 90,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    ...(clickable && {
        '&:hover': {
            boxShadow: theme.shadows[4],
            transform: 'translateY(-2px)',
        },
    }),
}));

interface AgentNodeProps {
    agentType?: AgentType;
    active?: boolean;
    label?: string;
    onClick?: () => void;
}

const AgentNode: React.FC<AgentNodeProps> = ({
    agentType = 'claude-code',
    active = true,
    label,
    onClick,
}) => {
    const { i18n } = useTranslation();
    const lang: Lang = i18n.language.startsWith('zh') ? 'zh' : 'en';
    const config = AGENT_TYPE_CONFIG[agentType] ?? AGENT_TYPE_CONFIG['mock'];
    const info = config.info[lang];
    const displayLabel = label || config.label;
    const clickable = !!onClick;

    const anchorEl = useRef<HTMLDivElement | null>(null);
    const [open, setOpen] = useState(false);
    const enterTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

    const handleMouseEnter = useCallback(() => {
        enterTimer.current = setTimeout(() => setOpen(true), 400);
    }, []);

    const handleMouseLeave = useCallback(() => {
        if (enterTimer.current) clearTimeout(enterTimer.current);
        setOpen(false);
    }, []);

    return (
        <>
            <StyledAgentNode
                ref={anchorEl}
                active={active}
                clickable={clickable}
                onClick={onClick}
                onMouseEnter={handleMouseEnter}
                onMouseLeave={handleMouseLeave}
            >
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Typography variant="body2" sx={NODE_LAYER_STYLES.typography}>Agent</Typography>
                </Box>

                <Divider sx={NODE_LAYER_STYLES.divider} />

                <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                    <Chip
                        label={displayLabel}
                        size="small"
                        color={config.color as any}
                        sx={{ height: 24, fontSize: '0.75rem', fontWeight: 600 }}
                    />
                </Box>
            </StyledAgentNode>

            <Popover
                open={open}
                anchorEl={anchorEl.current}
                onClose={() => setOpen(false)}
                anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'center' }}
                disableRestoreFocus
                slotProps={{ paper: { onMouseEnter: handleMouseEnter, onMouseLeave: handleMouseLeave } }}
                sx={{ pointerEvents: 'none' }}
            >
                <Paper sx={{ p: 2, maxWidth: 300, pointerEvents: 'auto' }}>
                    {/* Header */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                        <Chip
                            label={config.label}
                            size="small"
                            color={config.color as any}
                            sx={{ fontWeight: 700, fontSize: '0.75rem' }}
                        />
                        <Typography variant="caption" color="text.secondary">Agent</Typography>
                    </Box>

                    {/* Description */}
                    <Typography variant="body2" sx={{ mb: 1.5, lineHeight: 1.55, color: 'text.primary' }}>
                        {info.description}
                    </Typography>

                    <Divider sx={{ mb: 1.5 }} />

                    {/* Features */}
                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                        {lang === 'zh' ? '功能' : 'Features'}
                    </Typography>
                    <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2.5, mb: 1.5 }}>
                        {info.features.map((f) => (
                            <Box component="li" key={f} sx={{ mb: 0.25 }}>
                                <Typography variant="caption" color="text.secondary">{f}</Typography>
                            </Box>
                        ))}
                    </Box>

                    <Divider sx={{ mb: 1.5 }} />

                    {/* Config hint */}
                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                        {lang === 'zh' ? '配置' : 'Configuration'}
                    </Typography>
                    <Typography variant="caption" display="block" sx={{ mt: 0.5, color: 'text.secondary', lineHeight: 1.5 }}>
                        {info.config}
                    </Typography>
                </Paper>
            </Popover>
        </>
    );
};

export default AgentNode;
