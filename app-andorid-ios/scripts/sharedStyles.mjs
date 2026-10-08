export function sharedStyleTags(html) {
  const block = html.match(
    /<!-- reference-styles:start -->([\s\S]*?)<!-- reference-styles:end -->/,
  );
  if (!block)
    throw new Error("Shared index.html has no reference-styles markers");
  const hrefs = [
    ...block[1].matchAll(
      /<link\s+rel="stylesheet"\s+href="([^"]+)"\s*\/?\s*>/g,
    ),
  ].map((match) => match[1]);
  if (
    !hrefs.length ||
    hrefs.some((href) => !href.startsWith("/") || href.startsWith("//"))
  ) {
    throw new Error("Invalid local reference-styles manifest");
  }
  return hrefs.map((href) => ({
    tag: "link",
    attrs: { rel: "stylesheet", href },
    injectTo: "head",
  }));
}
