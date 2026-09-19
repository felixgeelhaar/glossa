/**
 * The e2e stack, provisioned the way production is (platform/README.md):
 * Postgres 16 in a testcontainer, a CREATEROLE owner that runs the
 * migrations, the server connecting as the non-superuser `glossa_app`,
 * and the log mailer, whose captured sign-in links the tests follow.
 */
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { randomBytes } from "node:crypto";
import { createWriteStream, existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { PostgreSqlContainer, type StartedPostgreSqlContainer } from "@testcontainers/postgresql";

const here = dirname(fileURLToPath(import.meta.url));
export const STATE_DIR = join(here, ".state");
export const SERVER_LOG = join(STATE_DIR, "server.log");
const PLATFORM = resolve(here, "../../platform");

export const ports = {
  api: Number(process.env.GLOSSA_E2E_API_PORT ?? 18317),
  studio: Number(process.env.GLOSSA_E2E_STUDIO_PORT ?? 4317),
};
export const studioURL = `http://localhost:${ports.studio}`;
export const apiURL = `http://127.0.0.1:${ports.api}`;

const OWNER_PASSWORD = "owner-e2e";
const APP_PASSWORD = "app-e2e";

async function psql(pg: StartedPostgreSqlContainer, user: string, password: string, sql: string): Promise<void> {
  const r = await pg.exec(["psql", "-v", "ON_ERROR_STOP=1", "-U", user, "-d", "glossa", "-c", sql], { env: { PGPASSWORD: password } });
  if (r.exitCode !== 0) throw new Error(`psql failed (${sql}): ${r.output}`);
}

function buildServer(): string {
  const bin = join(STATE_DIR, process.platform === "win32" ? "glossa-server.exe" : "glossa-server");
  if (process.env.GLOSSA_E2E_SERVER_BIN) return process.env.GLOSSA_E2E_SERVER_BIN;
  const r = spawnSync("go", ["build", "-o", bin, "./cmd/glossa-server"], { cwd: PLATFORM, stdio: "inherit" });
  if (r.status !== 0) throw new Error("go build ./cmd/glossa-server failed");
  return bin;
}

async function waitFor(url: string, server: ChildProcess, timeoutMs = 60_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (server.exitCode !== null) throw new Error(`glossa-server exited with ${server.exitCode}; see ${SERVER_LOG}`);
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`${url} not ready after ${timeoutMs} ms; see ${SERVER_LOG}`);
}

export async function startStack(): Promise<() => Promise<void>> {
  mkdirSync(STATE_DIR, { recursive: true });
  const bin = buildServer();

  const pg = await new PostgreSqlContainer("postgres:16-alpine").withDatabase("glossa").withUsername("postgres").withPassword("postgres").start();
  await psql(pg, "postgres", "postgres", `CREATE ROLE glossa_owner LOGIN CREATEROLE NOSUPERUSER NOBYPASSRLS PASSWORD '${OWNER_PASSWORD}'`);
  await psql(pg, "postgres", "postgres", "ALTER DATABASE glossa OWNER TO glossa_owner");

  const host = pg.getHost();
  const port = pg.getPort();
  const dsn = (user: string, password: string) => `postgres://${user}:${password}@${host}:${port}/glossa?sslmode=disable`;
  const env = {
    ...process.env,
    DATABASE_URL: dsn("glossa_app", APP_PASSWORD),
    MIGRATION_DATABASE_URL: dsn("glossa_owner", OWNER_PASSWORD),
    GLOSSA_AUTH_SECRET: randomBytes(32).toString("base64"),
    GLOSSA_HTTP_ADDR: `127.0.0.1:${ports.api}`,
    GLOSSA_STUDIO_URL: studioURL,
    GLOSSA_MAIL_DRIVER: "log",
    GLOSSA_LOG_LEVEL: "info",
    GLOSSA_OUTBOX_POLL_INTERVAL: "100ms",
    GLOSSA_SHUTDOWN_TIMEOUT: "5s",
  };

  const migrate = spawnSync(bin, ["-migrate=only"], { env, encoding: "utf8" });
  if (migrate.status !== 0) throw new Error(`migration failed:\n${migrate.stdout}\n${migrate.stderr}`);
  await psql(pg, "glossa_owner", OWNER_PASSWORD, `ALTER ROLE glossa_app LOGIN PASSWORD '${APP_PASSWORD}'`);

  const log = createWriteStream(SERVER_LOG, { flags: "w" });
  const server = spawn(bin, [], { env, stdio: ["ignore", "pipe", "pipe"] });
  server.stdout?.pipe(log);
  server.stderr?.pipe(log);
  await waitFor(`${apiURL}/readyz`, server);

  return async () => {
    server.kill("SIGTERM");
    await new Promise((r) => (server.exitCode !== null ? r(null) : server.once("exit", r)));
    log.end();
    await pg.stop();
  };
}

/** The newest sign-in link the log mailer captured for `email`. */
export async function signInLink(email: string, timeoutMs = 10_000): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  const re = /(https?:\/\/[^\s"\\]+\/auth\/sign-in#token=[A-Za-z0-9_-]+)/g;
  while (Date.now() < deadline) {
    if (existsSync(SERVER_LOG)) {
      const lines = readFileSync(SERVER_LOG, "utf8").split("\n").filter((l) => l.includes(email) && l.includes("#token="));
      const last = lines.at(-1);
      const links = last ? [...last.matchAll(re)].map((m) => m[1]!) : [];
      if (links.length) return links.at(-1)!;
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`no sign-in link for ${email} in ${SERVER_LOG}`);
}
