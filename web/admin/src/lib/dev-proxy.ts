// For `astro dev` only: admin's local proxy to the API, beyond what Vite's does by itself.
// astro.config.mjs loads it with Node, which strips types: keep to annotations (no enum, no namespace).

type ApiAnswer = { complete: boolean; on(event: 'close', listener: () => void): unknown };
type BrowserAnswer = { destroy(): unknown };

/** Proxy is the part of Vite's dev proxy that passBreaksOn uses. */
export type Proxy = {
  on(event: 'proxyRes', listener: (api: ApiAnswer, req: unknown, browser: BrowserAnswer) => void): unknown;
};

/** passBreaksOn makes proxy break the browser's answer off when the API's breaks off part way, rather than leave it open. */
export function passBreaksOn(proxy: Proxy): void {
  proxy.on('proxyRes', (api, _req, browser) => {
    api.on('close', () => {
      if (!api.complete) browser.destroy();
    });
  });
}
