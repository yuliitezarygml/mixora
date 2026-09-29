function isHttpProxyPath(pathname) {
  return (
    pathname.startsWith("/api/v1/") ||
    pathname === "/health" ||
    pathname.startsWith("/health/")
  );
}

module.exports = { isHttpProxyPath };
