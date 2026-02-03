import AnimatedBackground from "@/components/AnimatedBackground";
import { faqs, features, screenshots } from "@/data/text";
import { Card, CardContent, Dialog, DialogContent, Button as MuiButton } from "@mui/material";
import {
    ChevronLeft,
    ChevronRight,
    ExternalLink,
    X
} from "lucide-react";
import { useState } from "react";
import { FaGithub } from "react-icons/fa";
import {STEPS} from "@/data/code.tsx";

// Layout Constants
const SECTION_WIDTH = 1060;
const WIDTH_SCALE = 1;
const SCALED_WIDTH = `${SECTION_WIDTH * WIDTH_SCALE}px`; // 1060px (Hero image - full width)
const FULL_WIDTH = `${SECTION_WIDTH}px`; // 1060px (Content sections)


const Hero = () => {
    const [currentSlide, setCurrentSlide] = useState(0);
    const [previewOpen, setPreviewOpen] = useState(false);
    const [previewImage, setPreviewImage] = useState(0);

    const handleImageClick = (index: number) => {
        setPreviewImage(index);
        setPreviewOpen(true);
    };

    return (
        <>
            <section className="min-h-[60vh] flex flex-col items-center justify-center text-center px-4 py-16 sm:py-20">
                <h1 className="text-4xl sm:text-5xl md:text-6xl lg:text-7xl font-bold mb-6">
                    <span className="text-primary">Tingly</span> Box
                </h1>
                <p className="text-lg sm:text-xl md:text-2xl text-muted-foreground max-w-3xl mb-10 px-4">
                    Your local AI intelligence layer — an autonomous orchestrator that decides <span className="text-primary font-semibold">which model to call</span>, <span className="text-primary font-semibold">when to compress context</span>, and <span className="text-primary font-semibold">how to route requests</span> for maximum efficiency
                </p>
                <div className="flex flex-wrap justify-center gap-3 sm:gap-4 mb-8 sm:mb-10">
                    <MuiButton
                        variant="contained"
                        href="https://github.com/tingly-dev/tingly-box"
                        target="_blank"
                        rel="noopener noreferrer"
                        size="medium"
                        sx={{ gap: '8px', fontSize: { xs: '0.875rem', sm: '1rem' } }}
                    >
                        <FaGithub className="w-4 h-4 sm:w-5 sm:h-5" />
                        GitHub
                    </MuiButton>
                    <MuiButton
                        variant="outlined"
                        href="https://github.com/tingly-dev/tingly-box/releases"
                        target="_blank"
                        rel="noopener noreferrer"
                        size="medium"
                        sx={{ gap: '8px', fontSize: { xs: '0.875rem', sm: '1rem' } }}
                    >
                        <ExternalLink className="w-4 h-4 sm:w-5 sm:h-5" />
                        Releases
                    </MuiButton>
                </div>

                {/* Application of SCALED_WIDTH only to the carousel */}
                <div className="relative w-full mx-auto px-4" style={{ maxWidth: SCALED_WIDTH }}>
                    <div className="relative overflow-hidden rounded-2xl">
                        <img
                            src={screenshots[currentSlide].src}
                            alt={screenshots[currentSlide].alt}
                            className="w-full h-auto transition-opacity duration-300 cursor-pointer hover:opacity-90 rounded-2xl shadow-xl"
                            onClick={() => handleImageClick(currentSlide)}
                        />
                        <button
                            onClick={() => setCurrentSlide((prev) => (prev - 1 + screenshots.length) % screenshots.length)}
                            className="absolute left-2 sm:left-3 top-1/2 -translate-y-1/2 p-1.5 sm:p-2 rounded-full bg-background/90 border border-white/10 backdrop-blur-sm hover:bg-background transition-colors"
                        >
                            <ChevronLeft className="w-4 h-4 sm:w-5 sm:h-5" />
                        </button>
                        <button
                            onClick={() => setCurrentSlide((prev) => (prev + 1) % screenshots.length)}
                            className="absolute right-2 sm:right-3 top-1/2 -translate-y-1/2 p-1.5 sm:p-2 rounded-full bg-background/90 border border-white/10 backdrop-blur-sm hover:bg-background transition-colors"
                        >
                            <ChevronRight className="w-4 h-4 sm:w-5 sm:h-5" />
                        </button>
                    </div>
                    <div className="flex justify-center gap-1.5 sm:gap-2 mt-3 sm:mt-4">
                        {screenshots.map((_, i) => (
                            <button
                                key={i}
                                onClick={() => setCurrentSlide(i)}
                                className={`w-1.5 h-1.5 sm:w-2 sm:h-2 rounded-full transition-all ${i === currentSlide ? "bg-primary w-3 sm:w-4" : "bg-muted"}`}
                            />
                        ))}
                    </div>
                </div>
            </section>

            {/* Preview Dialog */}
            <Dialog
                open={previewOpen}
                onClose={() => setPreviewOpen(false)}
                maxWidth={false}
                fullWidth
                sx={{
                    '& .MuiDialog-paper': {
                        backgroundColor: 'transparent',
                        boxShadow: 'none',
                        overflow: 'hidden',
                        margin: 0,
                        maxWidth: '100vw',
                        width: '100vw'
                    }
                }}
            >
                <DialogContent sx={{ p: 0, position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <button
                        onClick={() => setPreviewOpen(false)}
                        className="absolute top-4 right-4 z-10 p-2 rounded-full bg-black/50 text-white hover:bg-black/70 transition-colors"
                    >
                        <X className="w-6 h-6" />
                    </button>
                    <img
                        src={screenshots[previewImage].src}
                        alt={screenshots[previewImage].alt}
                        className="max-w-[95vw] max-h-[95vh] w-auto h-auto object-contain"
                    />
                    <div className="absolute bottom-4 left-1/2 -translate-x-1/2 flex gap-2">
                        {screenshots.map((_, i) => (
                            <button
                                key={i}
                                onClick={() => setPreviewImage(i)}
                                className={`w-3 h-3 rounded-full transition-all ${i === previewImage ? "bg-white w-6" : "bg-white/50"}`}
                            />
                        ))}
                    </div>
                </DialogContent>
            </Dialog>
        </>
    );
};

const Features = () => (
    <section className="py-16 sm:py-20 px-4">
        {/* Background container with rounded corners */}
        <div className="mx-auto rounded-2xl bg-gradient-to-b from-card/60 to-card/80 backdrop-blur-sm shadow-xl border border-border/20"
            style={{ maxWidth: FULL_WIDTH, width: '100%' }}>
            <div className="px-4 sm:px-6 md:px-8 lg:px-12 py-12 sm:py-16">
                <h2 className="text-2xl sm:text-3xl font-bold text-center mb-8 sm:mb-12">Features</h2>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 sm:gap-6">
                    {features.map((f) => (
                        <Card key={f.title} sx={{
                            backgroundColor: 'var(--card)',
                            border: '1px solid var(--border)',
                            opacity: 1,
                            transition: 'all 0.2s ease-in-out',
                            '&:hover': {
                                boxShadow: 'var(--shadow-lg)',
                                opacity: 1.05,
                                transform: 'translateY(-2px)',
                            }
                        }}>
                            <CardContent sx={{ p: { xs: 2.5, sm: 3 } }}>
                                <f.icon className="w-8 h-8 sm:w-10 sm:h-10 text-primary mb-3 sm:mb-4" />
                                <h3 className="text-lg sm:text-xl font-semibold mb-2">{f.title}</h3>
                                <p className="text-muted-foreground text-sm sm:text-base">{f.description}</p>
                            </CardContent>
                        </Card>
                    ))}
                </div>
            </div>
        </div>
    </section>
);


const QuickStart = () => {
    return (
        <section className="py-16 sm:py-20 px-4">
            {/* Dark background container with limited width */}
            <div className="mx-auto rounded-2xl bg-gradient-to-b from-background/90 to-background/98 backdrop-blur-sm text-slate-200 shadow-2xl border border-border/30"
                style={{ maxWidth: FULL_WIDTH, width: '100%' }}>
                <div className="px-4 sm:px-6 md:px-8 lg:px-12 py-10 sm:py-12">
                    <h2 className="text-2xl sm:text-3xl font-bold text-center mb-12 sm:mb-16 tracking-tight text-white">Quick Start</h2>

                    <div className="relative">
                        {/* Vertical Center Line - Only show on md+ screens */}
                        <div className="absolute left-1/2 transform -translate-x-1/2 h-full w-px bg-gradient-to-b from-transparent via-slate-800 to-transparent hidden md:block" />

                        <div className="space-y-12 sm:space-y-16 md:space-y-20">
                            {STEPS.map((step, idx) => {
                                const isTextOnRight = idx % 2 === 0; // Step 1 (0), Step 3 (2)
                                const stepNum = idx + 1;

                                return (
                                    <div key={idx} className="relative flex flex-col md:flex-row items-center gap-4 md:gap-0">
                                        {/* Left Column */}
                                        <div className="w-full md:w-1/2 px-4 sm:px-6 md:px-8 lg:px-10 flex flex-col">
                                            {!isTextOnRight ? (
                                                /* Even Steps (2, 4): Text on Left, Number on Right near line on desktop */
                                                <>
                                                    {/* Mobile: Number above content */}
                                                    <div className="md:hidden w-full text-center mb-2">
                                                        <span className="w-8 h-8 bg-cyan-400 text-black rounded-full flex items-center justify-center font-bold shadow-[0_0_15px_rgba(34,211,238,0.4)] mx-auto">
                                                            {stepNum}
                                                        </span>
                                                    </div>
                                                    <div className="text-center md:text-right group w-full md:hidden mb-4">
                                                        <h3 className="text-xl font-bold group-hover:text-cyan-400 transition-colors text-white mb-2">
                                                            {step.title}
                                                        </h3>
                                                        <p className="text-slate-400 text-sm leading-relaxed">{step.desc}</p>
                                                    </div>
                                                    {/* Desktop: Text and Number aligned */}
                                                    <div className="hidden md:flex items-center justify-end gap-4 mb-3">
                                                        <div className="text-right group">
                                                            <h3 className="text-xl font-bold group-hover:text-cyan-400 transition-colors text-white">
                                                                {step.title}
                                                            </h3>
                                                            <p className="text-slate-400 text-sm leading-relaxed">{step.desc}</p>
                                                        </div>
                                                        <span className="w-8 h-8 flex-shrink-0 bg-cyan-400 text-black rounded-full flex items-center justify-center font-bold shadow-[0_0_15px_rgba(34,211,238,0.4)]">
                                                            {stepNum}
                                                        </span>
                                                    </div>
                                                </>
                                            ) : (
                                                /* Odd Steps (1, 3): Code on Left */
                                                <>
                                                    {/* Mobile: Number above code */}
                                                    <div className="md:hidden w-full text-center mb-2">
                                                        <span className="w-8 h-8 bg-cyan-400 text-black rounded-full flex items-center justify-center font-bold shadow-[0_0_15px_rgba(34,211,238,0.4)] mx-auto">
                                                            {stepNum}
                                                        </span>
                                                    </div>
                                                    <div className="w-full">
                                                        {step.content}
                                                    </div>
                                                </>
                                            )}
                                        </div>

                                        {/* Desktop Number for odd steps - positioned absolutely at center line */}
                                        {isTextOnRight && (
                                            <div className="hidden md:block absolute left-1/2 -translate-x-1/2">
                                                <span className="w-8 h-8 bg-cyan-400 text-black rounded-full flex items-center justify-center font-bold shadow-[0_0_15px_rgba(34,211,238,0.4)]">
                                                    {stepNum}
                                                </span>
                                            </div>
                                        )}

                                        {/* Right Column */}
                                        <div className="w-full md:w-1/2 px-4 sm:px-6 md:px-8 lg:px-10 flex flex-col mt-4 md:mt-0">
                                            {isTextOnRight ? (
                                                /* Odd Steps (1, 3): Text on Right */
                                                <>
                                                    {/* Mobile text content */}
                                                    <div className="text-center md:text-left group w-full md:hidden">
                                                        <h3 className="text-xl font-bold group-hover:text-cyan-400 transition-colors text-white mb-2">
                                                            {step.title}
                                                        </h3>
                                                        <p className="text-slate-400 text-sm leading-relaxed">{step.desc}</p>
                                                    </div>
                                                    {/* Desktop layout with number already handled above */}
                                                    <div className="hidden md:block text-left group w-full">
                                                        <h3 className="text-xl font-bold group-hover:text-cyan-400 transition-colors text-white mb-2">
                                                            {step.title}
                                                        </h3>
                                                        <p className="text-slate-400 text-sm leading-relaxed">{step.desc}</p>
                                                    </div>
                                                </>
                                            ) : (
                                                /* Even Steps (2, 4): Visual/Link on Right */
                                                <div className="w-full">
                                                    {step.content}
                                                </div>
                                            )}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                </div>
            </div>
        </section>
    );
};

const Footer = () => (
    <footer className="py-8 sm:py-12 px-4 border-t">
        <div className="mx-auto flex flex-col sm:flex-row items-center justify-between gap-4"
            style={{ maxWidth: FULL_WIDTH, width: '100%' }}>
            <div className="text-muted-foreground text-sm text-center sm:text-left">MPL-2.0 License © {new Date().getFullYear()} Tingly Box</div>
            <MuiButton
                href="https://github.com/tingly-dev/tingly-box"
                target="_blank"
                rel="noopener noreferrer"
                startIcon={<FaGithub className="w-4 h-4 sm:w-5 sm:h-5" />}
                sx={{
                    color: 'var(--muted-foreground)',
                    '&:hover': {
                        color: 'var(--foreground)',
                    }
                }}
            >
                GitHub
            </MuiButton>
        </div>
    </footer>
);

const FAQ = () => {
    return (
        <section className="py-12 sm:py-16 px-4">
            <div id="faq-section" className="mx-auto rounded-2xl bg-gradient-to-t from-background via-background to-card/20 backdrop-blur-sm shadow-2xl border border-border/30"
                style={{ maxWidth: FULL_WIDTH, width: '100%' }}>
                <div className="px-4 sm:px-6 md:px-8 lg:px-12 py-10 sm:py-12">
                    <h2 className="text-2xl sm:text-3xl font-bold text-center mb-3 sm:mb-4">Frequently Asked Questions</h2>
                    <p className="text-center text-muted-foreground mb-8 sm:mb-12 max-w-2xl mx-auto px-4">
                        Common questions about Tingly Box
                    </p>

                    {/* Accordion-style FAQ */}
                    <div className="max-w-4xl mx-auto space-y-3 sm:space-y-4">
                        {faqs.map((faq, index) => (
                            <div key={index} className="border-b border-border/50 pb-3 sm:pb-4 last:border-0">
                                <h3 className="text-base sm:text-lg font-semibold mb-2 flex items-start gap-2 sm:gap-3">
                                    <span
                                        className="flex-shrink-0 w-5 h-5 sm:w-6 sm:h-6 bg-primary/10 text-primary rounded-full flex items-center justify-center text-xs sm:text-sm font-mono mt-0.5">
                                        {String(index + 1).padStart(2, '0')}
                                    </span>
                                    <span className="text-foreground leading-tight">{faq.question}</span>
                                </h3>
                                <p className="ml-7 sm:ml-9 text-muted-foreground leading-relaxed text-sm sm:text-base break-words">
                                    {faq.answer.split('\n').map((paragraph, i) => (
                                        <span key={i}>
                                            {paragraph.startsWith('http') || paragraph.includes('://') ? (
                                                <span className="break-all">{paragraph}</span>
                                            ) : (
                                                paragraph
                                            )}
                                            {i < faq.answer.split('\n').length - 1 && <br />}
                                        </span>
                                    ))}
                                </p>
                            </div>
                        ))}
                    </div>

                    {/* Additional help */}
                    <div className="mt-12 sm:mt-16 text-center">
                        <p className="text-base sm:text-lg mb-4">Still have questions?</p>
                        <div className="flex flex-wrap justify-center gap-3 sm:gap-4">
                            <MuiButton
                                variant="outlined"
                                href="https://github.com/tingly-dev/tingly-box/issues"
                                target="_blank"
                                rel="noopener noreferrer"
                                startIcon={<ExternalLink className="w-4 h-4" />}
                                size="small"
                                sx={{ fontSize: { xs: '0.875rem', sm: '1rem' } }}
                            >
                                Ask on GitHub
                            </MuiButton>
                        </div>
                    </div>
                </div>
            </div>
        </section>
    );
};

const Index = () => (
    <main className="min-h-screen relative">
        <AnimatedBackground />
        <Hero />
        <Features />
        <QuickStart />
        <FAQ />
        <Footer />
    </main>
);

export default Index;