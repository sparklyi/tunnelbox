// The bouncer widget is served by the Go process and loaded from index.html.
export {};

declare global {
  interface Window {
    bouncer?: {
      verify: () => Promise<unknown>;
      ready: Promise<unknown> | null;
      base: string;
    };
  }
}
