import { defineConfig } from "vite-plus";
import vue from "@vitejs/plugin-vue";
import { execSync } from "node:child_process";
import { readFileSync } from "node:fs";

function git(command: string): string {
  try {
    return execSync(`git ${command}`, { encoding: "utf8" }).trim();
  } catch {
    // The Toolforge build container may not expose a usable git checkout.
    return "";
  }
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

// https://viteplus.dev/config/
export default defineConfig({
  plugins: [vue()],
  fmt: {
    htmlWhitespaceSensitivity: "ignore",
    printWidth: 80,
  },
  lint: {
    ignorePatterns: ["dist/**"],
    options: {
      typeAware: true,
      // Type checking stays with `vue-tsc -b` (see the `build` script):
      // tsgolint has no Vue SFC support, so it cannot resolve `*.vue` imports.
      typeCheck: false,
    },
  },
  staged: {
    "*": "vp check --fix",
  },
});
