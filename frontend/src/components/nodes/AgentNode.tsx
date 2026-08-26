import { Box, Chip, Divider, Typography } from '@mui/material';
import { NODE_LAYER_STYLES, StyledBotGraphNode } from './styles';
import NodeTooltip from './NodeTooltip';
import { useTranslation } from 'react-i18next';
import { type AppLanguage, resolveLanguage } from '@/i18n';

type AgentType = 'claude-code' | 'smart-guide' | 'custom' | 'mock';

interface AgentInfo {
    description: string;
    features: string[];
    config: string;
}

// Bilingual content is intentionally co-located here rather than in the
// i18n locale files (zh.ts / en.ts). These strings are graph-node popover
// copy that is tightly coupled to the AgentNode component; externalising
// them would scatter context-specific copy across the global translation
// namespace without benefit. Do NOT migrate these strings to the locale
// files — use the Record<AppLanguage, AgentInfo> pattern below to add translations.
const AGENT_TYPE_CONFIG: Record<AgentType, {
    label: string;
    color: 'info' | 'success' | 'default' | 'warning';
    info: Record<AppLanguage, AgentInfo>;
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
                config: 'Click the Profile node to the right to route @cc through a Claude Code profile.',
            },
            zh: {
                description: '由 Claude Code CLI 驱动的全栈开发代理——功能实现、重构、测试、构建和 Git 操作。',
                features: [
                    '跨文件实现与重构',
                    '运行测试、构建与调试',
                    'Git 操作：提交、推送、变基',
                ],
                config: '点击右侧 Profile 节点，为 @cc 选择 Claude Code Profile。',
            },
            ru: {
                description: 'Полноценный агент разработки (Claude Code CLI) — реализация, рефакторинг, тесты, сборки и операции с Git в вашем локальном окружении.',
                features: [
                    'Реализация и рефакторинг по нескольким файлам',
                    'Запуск тестов, сборок и отладка',
                    'Операции с Git: коммит, push, rebase',
                ],
                config: 'Нажмите на узел Profile справа, чтобы направить @cc через профиль Claude Code.',
            },
            fa: {
                description: 'ایجنت توسعهٔ همه‌جانبه (Claude Code CLI) — پیاده‌سازی، بازآرایی، آزمون، ساخت و عملیات Git در محیط محلی شما.',
                features: [
                    'پیاده‌سازی و بازآرایی در چند فایل',
                    'اجرای آزمون‌ها، ساخت و اشکال‌زدایی',
                    'عملیات Git: کامیت، push، rebase',
                ],
                config: 'روی گرهٔ Profile کلیک کنید تا @cc از راه یک پروفایل Claude Code مسیریابی شود.',
            },
            ar: {
                description: 'وكيل تطوير متكامل ‏(Claude Code CLI) — تنفيذ وإعادة هيكلة واختبارات وبناء وعمليات Git في بيئتك المحلية.',
                features: [
                    'تنفيذ وإعادة هيكلة عبر عدة ملفات',
                    'تشغيل الاختبارات والبناء والتصحيح',
                    'عمليات Git: الالتزام والدفع وإعادة الأساس',
                ],
                config: 'انقر عقدة Profile لتوجيه ‎@cc عبر ملف تعريف Claude Code.',
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
            ru: {
                description: 'Помощник по навигации и координации (@tb) — изучает проект, отвечает на вопросы и вносит небольшие правки, а тяжёлую реализацию передаёт @cc.',
                features: [
                    'Изучение файлов и объяснение архитектуры',
                    'Точечные небольшие правки: конфиги, переменные окружения',
                    'Постоянная память (MEMORY.md)',
                ],
                config: 'Нажмите на узел Model, чтобы выбрать провайдера и модель.',
            },
            fa: {
                description: 'دستیار ناوبری و هماهنگی ‏(@tb) — پروژه را می‌کاود، به پرسش‌ها پاسخ می‌دهد و تغییرهای کوچک را انجام می‌دهد و پیاده‌سازی سنگین را به @cc می‌سپارد.',
                features: [
                    'کاوش فایل‌ها و توضیح معماری',
                    'تغییرهای کوچک و دقیق: تنظیمات، متغیرهای محیطی',
                    'حافظهٔ ماندگار (MEMORY.md)',
                ],
                config: 'روی گرهٔ Model کلیک کنید تا ارائه‌دهنده و مدل را انتخاب کنید.',
            },
            ar: {
                description: 'مساعد للتنقُّل والتنسيق ‏(@tb) — يستكشف المشروع ويجيب عن الأسئلة وينفِّذ التعديلات الصغيرة، ويحيل التنفيذ الثقيل إلى ‎@cc.',
                features: [
                    'استكشاف الملفات وشرح البنية',
                    'تعديلات صغيرة دقيقة: الإعدادات ومتغيرات البيئة',
                    'ذاكرة دائمة ‏(MEMORY.md)',
                ],
                config: 'انقر عقدة Model لاختيار المزوِّد والنموذج.',
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
            ru: {
                description: 'Собственная реализация агента с заданным вами поведением и эндпоинтами.',
                features: ['Своя обработка запросов и ответов', 'Свои интеграции инструментов'],
                config: 'Настраивается на панели настроек агента.',
            },
            fa: {
                description: 'پیاده‌سازی سفارشی ایجنت با رفتار و نشانی‌های دلخواه شما.',
                features: ['پردازش دلخواه درخواست و پاسخ', 'یکپارچگی‌های دلخواه با ابزارها'],
                config: 'از پنل تنظیمات ایجنت پیکربندی می‌شود.',
            },
            ar: {
                description: 'تنفيذ مخصص للوكيل بسلوك وعناوين تحدِّدها بنفسك.',
                features: ['معالجة مخصصة للطلبات والردود', 'تكاملات مخصصة مع الأدوات'],
                config: 'يُهيَّأ من لوحة إعدادات الوكيل.',
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
            ru: {
                description: 'Мок-агент для тестирования и разработки. Возвращает заранее заданные ответы без обращений к внешним API.',
                features: ['Заранее заданные тестовые ответы', 'Без обращений к внешним API', 'Удобен для тестирования интерфейса'],
                config: 'Настройка не требуется.',
            },
            fa: {
                description: 'ایجنت ساختگی برای آزمون و توسعه. پاسخ‌های از پیش تعیین‌شده برمی‌گرداند و هیچ فراخوانی بیرونی API انجام نمی‌دهد.',
                features: ['پاسخ‌های آزمایشی از پیش تعیین‌شده', 'بدون فراخوانی بیرونی API', 'مناسب برای آزمون رابط کاربری'],
                config: 'به پیکربندی نیاز ندارد.',
            },
            ar: {
                description: 'وكيل وهمي للاختبار والتطوير. يعيد ردودًا مُعدَّة مسبقًا دون أي استدعاءات خارجية لـ API.',
                features: ['ردود اختبار مُعدَّة مسبقًا', 'بلا استدعاءات خارجية لـ API', 'مفيد لاختبار الواجهة'],
                config: 'لا يحتاج إلى تهيئة.',
            },
        },
    },
};

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
    const lang = resolveLanguage(i18n.language);
    const config = AGENT_TYPE_CONFIG[agentType] ?? AGENT_TYPE_CONFIG['mock'];
    const info = config.info[lang];
    const displayLabel = label || config.label;
    const clickable = !!onClick;

    // NodeTooltip (MUI Tooltip) rather than a hand-rolled hover Popover: it
    // has built-in enter/leave hysteresis and never needs to reposition
    // itself under the cursor, so it can't fall into the open/close flicker
    // loop a manually-timed Popover does when its content is tall enough to
    // collide with the viewport edge.
    // Typography variants in this app's theme carry a fixed color (body2 →
    // text.secondary, caption → text.disabled) meant for normal page
    // backgrounds. Left alone inside the Tooltip's own dark bubble, that
    // reads as an unintentionally washed-out grey, so every line here
    // explicitly inherits the Tooltip's own text color instead.
    const tooltipContent = (
        <Box sx={{ maxWidth: 260 }}>
            <Typography variant="body2" sx={{ mb: 1, lineHeight: 1.5, color: 'inherit' }}>
                {info.description}
            </Typography>
            <Box component="ul" sx={{ m: 0, pl: 2.25, mb: 1 }}>
                {info.features.map((f) => (
                    <Box component="li" key={f} sx={{ '&:not(:last-of-type)': { mb: 0.25 } }}>
                        <Typography variant="caption" sx={{ color: 'inherit' }}>{f}</Typography>
                    </Box>
                ))}
            </Box>
            <Divider sx={{ my: 0.75, borderColor: 'rgba(255,255,255,0.24)' }} />
            <Typography variant="caption" sx={{ display: 'block', fontStyle: 'italic', color: 'inherit' }}>
                {info.config}
            </Typography>
        </Box>
    );

    return (
        <NodeTooltip title={tooltipContent} placement="top">
            <StyledBotGraphNode active={active} clickable={clickable} onClick={onClick}>
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
            </StyledBotGraphNode>
        </NodeTooltip>
    );
};

export default AgentNode;
