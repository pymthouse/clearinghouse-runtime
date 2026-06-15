import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

// Proves `@pymthouse/builder-sdk` resolves end-to-end (types + ESM/CJS build
// outputs) from its `signer/webhook` subpath export. The `/authorize` route
// that wires this handler up is implemented in a follow-up issue.
import { createRemoteSignerAuthorizeHandler } from "@pymthouse/builder-sdk/signer/webhook";

export const PORT = Number(process.env.PORT ?? 8787);

export function handleRequest(req: IncomingMessage, res: ServerResponse): void {
  if (req.url === "/healthz") {
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({ status: "ok" }));
    return;
  }

  res.writeHead(404, { "content-type": "application/json" });
  res.end(JSON.stringify({ error: "not_found" }));
}

export function createApp() {
  return createServer(handleRequest);
}

export { createRemoteSignerAuthorizeHandler };

const isMain = process.argv[1] && import.meta.url === `file://${process.argv[1]}`;
if (isMain) {
  createApp().listen(PORT, () => {
    console.log(`clearinghouse listening on :${PORT}`);
  });
}
