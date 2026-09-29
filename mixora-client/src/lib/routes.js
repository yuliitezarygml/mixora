import routes from "../data/original-routes.json" with { type: "json" };
export function isDiscoveryPath(path) {
  return (
    path === "/mixes" ||
    path === "/playlists" ||
    path === "/tag" ||
    path.startsWith("/genre") ||
    path.startsWith("/chart") ||
    path.startsWith("/kids") ||
    path.startsWith("/non-music")
  );
}
export function pageKind(path) {
  if (path === "/") return "home";
  if (path === "/search") return "search";
  if (path === "/settings") return "settings";
  if (path.startsWith("/collection") || path.startsWith("/mymusic"))
    return "collection";
  if (
    path.startsWith("/artist") ||
    path.startsWith("/album") ||
    path.startsWith("/label") ||
    path === "/playlist" ||
    /\/editorial\/(album|playlist)$/.test(path)
  )
    return "details";
  if (["/404", "/not-found", "/unavailable"].includes(path)) return "error";
  if (path === "/oauth") return "oauth";
  if (["/verify-email", "/reset-password"].includes(path)) return "auth_action";
  if (path === "/landing") return "landing";
  return routes.includes(path) ? "browse" : "error";
}
export const originalRoutes = routes;
