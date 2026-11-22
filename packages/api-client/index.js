/**
 * Minimal earnings-sync HTTP client (browser / Node 18+).
 * Contract: docs/api-contract.md · packages/api-types/openapi-lite.json
 */

/**
 * @typedef {object} ClientOptions
 * @property {string} [baseURL]
 * @property {() => string} [getToken]
 * @property {typeof fetch} [fetch]
 */

/**
 * @param {ClientOptions} [opts]
 */
export function createClient(opts = {}) {
  const fetchImpl = opts.fetch || globalThis.fetch;
  if (typeof fetchImpl !== "function") {
    throw new Error("fetch is required");
  }

  function base() {
    return String(opts.baseURL || "http://127.0.0.1:8090").replace(/\/$/, "");
  }

  function headers(extra) {
    const h = { Accept: "application/json", ...(extra || {}) };
    const tok = typeof opts.getToken === "function" ? opts.getToken() : "";
    if (tok) h.Authorization = "Bearer " + tok;
    return h;
  }

  async function request(path, init) {
    const res = await fetchImpl(base() + path, init);
    const text = await res.text();
    let data = null;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = { raw: text };
      }
    }
    if (!res.ok) {
      const err = new Error((data && data.error) || res.statusText || String(res.status));
      err.status = res.status;
      err.body = data;
      throw err;
    }
    return data;
  }

  return {
    baseURL: base,
    meta: () => request("/v1/meta", { headers: headers() }),
    authMe: () => request("/v1/auth/me", { headers: headers() }),
    authProviders: () => request("/v1/auth/providers", { headers: headers() }),
    platformStatus: () => request("/v1/platform/status", { headers: headers() }),
    /**
     * @param {string} ticker
     */
    facts: (ticker) =>
      request("/v1/facts?ticker=" + encodeURIComponent(String(ticker || "").toUpperCase()), {
        headers: headers(),
      }),
  };
}

export default createClient;
