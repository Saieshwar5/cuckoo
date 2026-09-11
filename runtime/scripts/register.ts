/**
 * Put a ready-made agent on the hub and point it at this runtime.
 *
 * The runtime creates agents through the management API like any company
 * would — there is no other way in, on purpose. Run it once per agent; running
 * it again re-points an existing one, which is what moving the runtime to a
 * new address needs.
 *
 *   RUNTIME_HUB_TOKEN=ses_tok_… or mgt_tok_…  \
 *   RUNTIME_DATABASE_URL=… RUNTIME_SECRET_KEY=… \
 *   node --experimental-strip-types scripts/register.ts weather
 */

import { Registry } from "../src/agents/registry.ts";
import { loadConfig } from "../src/config.ts";
import { migrate, openPool } from "../src/db/pool.ts";
import { TEMPLATES } from "../src/templates.ts";

const templateId = process.argv[2] ?? "weather";
const template = TEMPLATES.find((t) => t.id === templateId);
if (!template) throw new Error(`no template called ${templateId}`);

const token = process.env.RUNTIME_HUB_TOKEN;
if (!token) throw new Error("set RUNTIME_HUB_TOKEN to a session or management token");

const config = loadConfig();
const handle = process.env.RUNTIME_AGENT_HANDLE ?? templateId;
// An agent whose template can keep routines holds schedules, and the app
// offers them for it.
const takesSchedules = template.tools.includes("create_routine");
const publicUrl = config.publicUrl || `http://localhost:${config.port}`;

const pool = openPool(config.databaseUrl);
await migrate(pool);
const registry = new Registry(pool, config.secretKey, config.defaultModel);
for (const t of TEMPLATES) await registry.putTemplate(t);

async function mgmt(
  path: string,
  body?: unknown,
  method = "POST",
): Promise<Record<string, unknown>> {
  const response = await fetch(`${config.hubUrl}/v1/mgmt${path}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`hub said ${response.status} to ${path}: ${text.slice(0, 300)}`);
  return text ? (JSON.parse(text) as Record<string, unknown>) : {};
}

// Find it if it is already there: the hub keeps a handle for the life of the
// hub, so creating one twice is an error rather than a second agent.
const listed = (await mgmt("/agents", undefined, "GET")).agents as
  | { id: string; handle: string }[]
  | undefined;
let hubAgentId = listed?.find((a) => a.handle === handle)?.id;

if (hubAgentId) {
  console.log(`@${handle} already exists: ${hubAgentId}`);
  // It may predate the catalogue and schedules, so say what it is now.
  await mgmt(`/agents/${hubAgentId}`, { listed: true, supports_schedules: takesSchedules }, "PATCH");
} else {
  const created = (await mgmt("/agents", {
    handle,
    display_name: template.name,
    description: `${template.name}, run by Cuckoo.`,
    starters: template.starters,
    // A ready-made agent is offered in the catalogue; that is what it is for.
    // A custom one, made for one person, is private instead and gets neither
    // a listing nor a code.
    listed: true,
    supports_schedules: takesSchedules,
  })) as { agent: { id: string } };
  hubAgentId = created.agent.id;
  console.log(`created @${handle}: ${hubAgentId}`);
}

// A webhook binding, because one URL serves every agent. The hub refuses
// anything that is not HTTPS in production, and allows loopback in dev.
const bound = (await mgmt(`/agents/${hubAgentId}/binding`, {
  mode: "webhook",
  webhook_url: `${publicUrl}/hooks/${hubAgentId}`,
})) as { secret?: string };
if (!bound.secret) throw new Error("the hub returned no secret; a binding shows it once");

await registry.register({
  hubAgentId,
  ownerUserId: process.env.RUNTIME_OWNER_USER_ID ?? "usr_cuckoo",
  templateId: template.id,
  bindingSecret: bound.secret,
  private: false,
});
console.log(`bound → ${publicUrl}/hooks/${hubAgentId}`);

await pool.end();
