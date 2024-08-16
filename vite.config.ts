import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { execSync } from "node:child_process";
import { readFileSync } from "node:fs";

function git(command: string): string {
  return execSync(`git ${command}`, { encoding: "utf8" }).trim();
}

function json(path: string) {
  return JSON.parse(readFileSync(path, { encoding: "utf8" }));
}

process.env.VITE_BUILD_DATE = git("log -1 --format=%cd --date=short");
process.env.VITE_BUILD_VERSION = git("describe --always");
process.env.VITE_APP_DEPENDENCIES = Object.keys(
  json("./package.json").dependencies,
)
  .map((dependency) => {
    const { name, version, license } = json(
      `node_modules/${dependency}/package.json`,
    );
    return `${name} ${version} (${license})`;
  })
  .join(", ");

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
});
