#!/usr/bin/env node
import { createTenantRegistry } from "./registry.mjs";

function usage() {
  console.log(`Usage:
  node cli.mjs provision-tenant \\
    --tenant-id <id> --tenant-name <name> \\
    [--client-id <billing-client-id>] [--auth0-client-id <auth0-public-client-id>]

  node cli.mjs register-auth0 \\
    --tenant-id <id> --client-id <billing-client-id> \\
    --auth0-client-id <auth0-public-client-id> [--tenant-name <name>]

  node cli.mjs list

  node cli.mjs sync-auth0-bootstrap \\
    --bootstrap-env <path-to-.env.livepeer> \\
    --tenant-id <id> --client-id <billing-client-id> [--tenant-name <name>]
`);
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (token.startsWith("--")) {
      const key = token.slice(2).replace(/-/g, "_");
      const next = argv[i + 1];
      if (!next || next.startsWith("--")) {
        args[key] = true;
      } else {
        args[key] = next;
        i += 1;
      }
    } else {
      args._.push(token);
    }
  }
  return args;
}

function parseEnvFile(content) {
  const out = {};
  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) {
      continue;
    }
    const eq = trimmed.indexOf("=");
    if (eq <= 0) {
      continue;
    }
    out[trimmed.slice(0, eq)] = trimmed.slice(eq + 1);
  }
  return out;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const command = args._[0];
  if (!command || args.help) {
    usage();
    process.exit(command ? 0 : 1);
  }

  const registry = createTenantRegistry({
    dataDir: process.env.TENANT_ADMIN_DATA_DIR?.trim(),
  });

  switch (command) {
    case "provision-tenant": {
      const result = await registry.provisionTenant({
        tenantId: args.tenant_id,
        tenantName: args.tenant_name,
        clientId: args.client_id,
        auth0ClientId: args.auth0_client_id,
      });
      console.log(JSON.stringify(result, null, 2));
      return;
    }
    case "register-auth0": {
      const app = await registry.registerAuth0App({
        tenantId: args.tenant_id,
        tenantName: args.tenant_name,
        clientId: args.client_id,
        auth0ClientId: args.auth0_client_id,
      });
      console.log(JSON.stringify({ app }, null, 2));
      return;
    }
    case "list": {
      const tenants = await registry.listTenants();
      const apps = await registry.listApps();
      console.log(JSON.stringify({ tenants, apps }, null, 2));
      return;
    }
    case "sync-auth0-bootstrap": {
      const { readFile } = await import("node:fs/promises");
      const envPath = args.bootstrap_env;
      if (!envPath) {
        throw new Error("--bootstrap-env is required");
      }
      const env = parseEnvFile(await readFile(envPath, "utf8"));
      const auth0ClientId = env.AUTH0_PUBLIC_CLIENT_ID?.trim();
      if (!auth0ClientId) {
        throw new Error("bootstrap env missing AUTH0_PUBLIC_CLIENT_ID");
      }
      const app = await registry.registerAuth0App({
        tenantId: args.tenant_id,
        tenantName: args.tenant_name ?? args.tenant_id,
        clientId: args.client_id,
        auth0ClientId,
      });
      console.log(
        JSON.stringify(
          {
            app,
            jwtIssuer: env.JWT_ISSUER ?? null,
            jwtAudience: env.JWT_AUDIENCE ?? null,
          },
          null,
          2,
        ),
      );
      return;
    }
    default:
      usage();
      process.exit(1);
  }
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : err);
  process.exit(1);
});
