// Compiles the shell into one executable for a platform package
// (scripts/pack.sh, docs/shell.md): the Bun runtime, the Solid transform
// and OpenTUI's native library inside, so the person needs no Bun.
//   bun build.ts <bun-linux-x64|bun-linux-arm64> <outfile>
// The native library of a target that is not this machine's is installed by
// `bun install --os=linux --cpu='*'` first; without it the build has none to
// put inside.

import solidPlugin from "@opentui/solid/bun-plugin";

const [target, outfile] = process.argv.slice(2);
if (!target || !outfile) {
  process.stderr.write("usage: bun build.ts <target> <outfile>\n");
  process.exit(2);
}

const result = await Bun.build({
  // The face runs as a worker of its own (src/face/faces.ts), so it is an
  // entrypoint beside the shell's.
  entrypoints: [`${import.meta.dir}/src/main.tsx`, `${import.meta.dir}/src/face/worker.ts`],
  // The shell runs in the person's directory: a bunfig.toml, .env, tsconfig
  // or package.json there must not reach it, as they would by default — a
  // preload named in one stopped the spike's executable.
  compile: {
    target: target as Bun.Build.CompileTarget,
    outfile,
    autoloadBunfig: false,
    autoloadDotenv: false,
    autoloadTsconfig: false,
    autoloadPackageJson: false,
  },
  plugins: [solidPlugin],
  minify: true,
});
if (!result.success) {
  for (const log of result.logs) process.stderr.write(`${log}\n`);
  process.exit(1);
}
