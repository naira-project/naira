export interface LogsConfig {
  /** Base URL of the Grafana Explore UI, without a trailing slash. */
  explorerUrl: string;
  datasource: { type: string; uid: string };
  /** Namespace for Services and the log proxy when the endpoint carries none. */
  defaultNamespace?: string;
  /** LogQL for a backend Service's pods. Placeholders: {{namespace}}, {{name}}. */
  serviceQuery: string;
  /** LogQL for the proxy's lines about a model. Placeholders: {{namespace}}, {{model}}. */
  proxyQuery: string;
}

export interface RuntimeConfig {
  /** Absent when no log explorer is configured; the UI then offers no logs link. */
  logs?: LogsConfig;
}

/** Loads the deployment's `/config.json`; an unreadable file yields an empty config. */
export async function fetchRuntimeConfig(): Promise<RuntimeConfig> {
  try {
    const response = await fetch('/config.json', { cache: 'no-store' });
    if (!response.ok) {
      return {};
    }
    return (await response.json()) as RuntimeConfig;
  } catch {
    return {};
  }
}
