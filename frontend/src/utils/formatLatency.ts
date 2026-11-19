// formatLatency renders a millisecond latency compactly: "850ms" / "1.8s".
export const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);
