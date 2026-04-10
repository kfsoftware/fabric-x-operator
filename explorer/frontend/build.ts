await Bun.build({
  entrypoints: ["./src/main.tsx"],
  outdir: "./dist",
  minify: true,
  splitting: false,
  target: "browser",
  format: "esm",
});

// Copy index.html to dist
const html = await Bun.file("./index.html").text();
await Bun.write("./dist/index.html", html);

console.log("Build complete → dist/");
