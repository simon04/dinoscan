// Static file server for the built SPA, used by the Toolforge build service.
// Deliberately dependency-free: the Node.js buildpack prunes devDependencies
// after `pnpm build`, so anything needed at runtime would have to be a
// production dependency.
import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { createServer } from "node:http";
import { extname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("dist", import.meta.url)));
const port = Number(process.env.PORT) || 8000;

const contentTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".ico": "image/vnd.microsoft.icon",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".txt": "text/plain; charset=utf-8",
  ".woff2": "font/woff2",
};

/** Resolves a request path to a readable file below `dist`, or `undefined`. */
async function resolveFile(pathname) {
  let decoded;
  try {
    decoded = decodeURIComponent(pathname);
  } catch {
    return undefined;
  }
  const candidate = resolve(join(root, decoded));
  if (candidate !== root && !candidate.startsWith(root + sep)) return undefined;
  const stats = await stat(candidate).catch(() => undefined);
  return stats?.isFile() ? candidate : undefined;
}

createServer(async (request, response) => {
  if (request.method !== "GET" && request.method !== "HEAD") {
    response.writeHead(405, { allow: "GET, HEAD" }).end();
    return;
  }

  const { pathname } = new URL(request.url, "http://localhost");
  // Vite emits content-hashed assets, everything else is the SPA entry point.
  const file = (await resolveFile(pathname)) ?? join(root, "index.html");
  const immutable = file.startsWith(join(root, "assets") + sep);

  response.writeHead(200, {
    "cache-control": immutable
      ? "public, max-age=31536000, immutable"
      : "no-cache",
    "content-type": contentTypes[extname(file)] ?? "application/octet-stream",
  });
  if (request.method === "HEAD") {
    response.end();
    return;
  }
  createReadStream(file)
    .on("error", () => response.destroy())
    .pipe(response);
}).listen(port, () => console.log(`Listening on port ${port}`));
