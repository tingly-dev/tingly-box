import { Box, Chip, Divider, Typography, styled } from '@mui/material';
import { NODE_LAYER_STYLES } from './styles';
import NodeTooltip from './NodeTooltip';
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
                description: 'A full-spectrum development agent (Claude Code CLI) — implementation, refactors, tests, builds, and git operations in your local environment.',
                features: [
                    'Multi-file implementation & refactors',
                    'Run tests, builds, and debug',
                    'Git operations: commit, push, rebase',
                ],
                config: 'Click to open Claude Code and configure profiles.',
            },
            zh: {
                description: '由 Claude Code CLI 驱动的全栈开发代理——功能实现、重构、测试、构建和 Git 操作。',
                features: [
                    '跨文件实现与重构',
                    '运行测试、构建与调试',
                    'Git 操作：提交、推送、变基',
                ],
                config: '点击跳转到 Claude Code 场景页配置 Profile。',
            },
        },
    },
    'smart-guide': {
        label: 'SmartGuide',
        color: 'success',
        info: {
            en: {
                description: 'A navigation and coordination assistant (@tb) — explores the project, answers questions, and handles small edits, then hands off heavy implementation to @cc.',
                features: [
                    'Explore files & explain architecture',
                    'Small precise edits: config, env vars',
                    'Persistent memory (MEMORY.md)',
                ],
                config: 'Click the Model node to select provider and model.',
            },
            zh: {
                description: '导航与协调助手（@tb）——探索项目、回答问题、处理小改动，重度实现交由 @cc。',
                features: [
                    '探索文件并讲解架构',
                    '精准小改动：配置、环境变量',
                    '跨会话持久记忆（MEMORY.md）',
                ],
                config: '点击右侧 Model 节点选择服务商和模型。',
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

    // NodeTooltip (MUI Tooltip) rather than a hand-rolled hover Popover: it
    // has built-in enter/leave hysteresis and never needs to reposition
    // itself under the cursor, so it can't fall into the open/close flicker
    // loop a manually-timed Popover does when its content is tall enough to
    // collide with the viewport edge.
    const tooltipContent = (
        <Box sx={{ maxWidth: 260 }}>
            <Typography variant="body2" sx={{ mb: 1, lineHeight: 1.5 }}>
                {info.description}
            </Typography>
            <Box component="ul" sx={{ m: 0, pl: 2.25, mb: 1 }}>
                {info.features.map((f) => (
                    <Box component="li" key={f} sx={{ '&:not(:last-of-type)': { mb: 0.25 } }}>
                        <Typography variant="caption">{f}</Typography>
                    </Box>
                ))}
            </Box>
            <Divider sx={{ my: 0.75, borderColor: 'rgba(255,255,255,0.2)' }} />
            <Typography variant="caption" sx={{ display: 'block', fontStyle: 'italic', opacity: 0.85 }}>
                {info.config}
            </Typography>
        </Box>
    );

    return (
        <NodeTooltip title={tooltipContent} placement="top">
            <StyledAgentNode active={active} clickable={clickable} onClick={onClick}>
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
        </NodeTooltip>
    );
};

export default AgentNode;
