/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly DEV: boolean;
  readonly MODE: string;
  readonly BASE_URL: string;
  readonly PROD: boolean;
  readonly SSR: boolean;
  /** Dev build commit stamped in by vite.config.ts ("dev-a1b2c3d"); empty in production builds. */
  readonly VITE_DEV_VERSION?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
