import { useThemeMode } from '../contexts/ThemeContext';
import { useEffect, useState } from 'react';
import { Z_INDEX } from '../constants/zIndex';

/**
 * Sunlit theme background component
 * Inspired by https://github.com/ugur-eren/sunlit
 * Features: dappled light, blinds, progressive blur, leaf billowing effect
 */
export const SunlitBackground: React.FC = () => {
    const { mode } = useThemeMode();
    const [animationReady, setAnimationReady] = useState(false);
    const isDark = mode === 'dark';

    useEffect(() => {
        // Trigger animation after mount
        const timer = setTimeout(() => setAnimationReady(true), 50);
        return () => clearTimeout(timer);
    }, []);

    return (
        <div
            id="sunlit-background"
            style={{
                position: 'fixed',
                top: 0,
                left: 0,
                width: '100vw',
                height: '100vh',
                pointerEvents: 'none',
                zIndex: Z_INDEX.sunlitBackground,
                opacity: animationReady ? 1 : 0,
                transition: 'opacity 0.5s ease',
            }}
        >
            {/* Glow effect */}
            <div
                style={{
                    position: 'absolute',
                    background: isDark
                        ? 'linear-gradient(355deg, #1b293f 0%, transparent 30%, transparent 100%)'
                        : 'linear-gradient(309deg, #f5d7a6, #f5d7a6 20%, transparent)',
                    transition: 'background 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                    height: '100%',
                    width: '100%',
                    opacity: isDark ? 0.5 : 0.4,
                }}
            />

            {/* Blinds effect */}
            <div
                style={{
                    position: 'absolute',
                    top: '-30vh',
                    right: 0,
                    width: '80vw',
                    height: '130vh',
                    opacity: isDark ? 0.25 : 0.12,
                    backgroundBlendMode: 'darken',
                    transformOrigin: 'top right',
                    transform: isDark
                        ? 'matrix3d(0.8333, 0.0833, 0.0000, 0.0003, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000)'
                        : 'matrix3d(0.7500, -0.0625, 0.0000, 0.0008, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000)',
                    transition: 'transform 1.7s cubic-bezier(0.455, 0.190, 0.000, 0.985), opacity 4s ease',
                }}
            >
                {/* Leaves with subtle animation */}
                <div
                    style={{
                        position: 'absolute',
                        backgroundSize: 'cover',
                        backgroundRepeat: 'no-repeat',
                        bottom: '-20px',
                        right: '-700px',
                        width: '1600px',
                        height: '1400px',
                        backgroundImage: 'url("/assets/leaves.png")',
                        opacity: 0.6,
                        animation: 'leafBillow 8s ease-in-out infinite',
                    }}
                />

                {/* Horizontal blinds */}
                <div
                    style={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'flex-end',
                        gap: isDark ? '20px' : '60px',
                        transition: 'gap 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                        width: '100%',
                    }}
                >
                    {Array.from({ length: 24 }).map((_, i) => (
                        <div
                            key={i}
                            style={{
                                width: '100%',
                                height: isDark ? '80px' : '40px',
                                backgroundColor: isDark ? '#030307' : '#1a1917',
                                transition: 'height 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                            }}
                        />
                    ))}
                </div>

                {/* Vertical bars */}
                <div
                    style={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        width: '100%',
                        height: '100%',
                        display: 'flex',
                        justifyContent: 'space-around',
                    }}
                >
                    {Array.from({ length: 3 }).map((_, i) => (
                        <div
                            key={i}
                            style={{
                                width: '5px',
                                height: '100%',
                                backgroundColor: isDark ? '#030307' : '#1a1917',
                            }}
                        />
                    ))}
                </div>
            </div>

            {/* Progressive blur layers */}
            <div
                style={{
                    position: 'absolute',
                    height: '100%',
                    width: '100%',
                }}
            >
                <div
                    style={{
                        position: 'absolute',
                        height: '100%',
                        width: '100%',
                        inset: 0,
                        backdropFilter: 'blur(6px)',
                        maskImage: 'linear-gradient(252deg, transparent, transparent 0%, black 0%, black)',
                    }}
                />
                <div
                    style={{
                        position: 'absolute',
                        height: '100%',
                        width: '100%',
                        inset: 0,
                        backdropFilter: 'blur(12px)',
                        maskImage: 'linear-gradient(252deg, transparent, transparent 40%, black 80%, black)',
                    }}
                />
                <div
                    style={{
                        position: 'absolute',
                        height: '100%',
                        width: '100%',
                        inset: 0,
                        backdropFilter: 'blur(48px)',
                        maskImage: 'linear-gradient(252deg, transparent, transparent 40%, black 70%, black)',
                    }}
                />
                <div
                    style={{
                        position: 'absolute',
                        height: '100%',
                        width: '100%',
                        inset: 0,
                        backdropFilter: 'blur(96px)',
                        maskImage: 'linear-gradient(252deg, transparent, transparent 70%, black 80%, black)',
                    }}
                />
            </div>

            <style>{`
                @keyframes leafBillow {
                    0% {
                        transform: perspective(400px) rotateX(0deg) rotateY(0deg) scale(1);
                    }
                    25% {
                        transform: perspective(400px) rotateX(1deg) rotateY(2deg) scale(1.02);
                    }
                    50% {
                        transform: perspective(400px) rotateX(-4deg) rotateY(-2deg) scale(0.97);
                    }
                    75% {
                        transform: perspective(400px) rotateX(1deg) rotateY(-1deg) scale(1.04);
                    }
                    100% {
                        transform: perspective(400px) rotateX(0deg) rotateY(0deg) scale(1);
                    }
                }
            `}</style>
        </div>
    );
};
