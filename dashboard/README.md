# Cold Harbour dashboard

The dashboard is the React and TypeScript UI for the Cold Harbour control plane.
It submits redaction and mask jobs, watches live job events, displays signed
results, creates delivery links, downloads compliance reports, and shows
Sentinel node status.

## Run locally

Start the control plane at `http://127.0.0.1:8081`, then run:

```sh
npm install
npm run dev
```

The Vite development server proxies `/v1` and `/d` to the control-plane target
configured in `vite.config.ts`. The browser logs in with an API key through
`/v1/session/login`; the response sets a session cookie used by subsequent
requests. Mutating cookie-authenticated requests include the returned CSRF
token, so the API key is not kept in the browser URL.

Use the Jobs view for redaction workflows and the Sentinel view for node
status and detection counters.
