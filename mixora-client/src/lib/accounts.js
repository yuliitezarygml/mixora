export function rememberAccount(list, user, _legacyToken) {
  if (!user?.id) return Array.isArray(list) ? list.slice(0, 8) : [];
  // Session credentials live only in the HttpOnly cookie. Mapping every saved
  // profile also removes tokens left by older desktop builds.
  const previous = Array.isArray(list)
    ? list.map(({ token: _legacyToken, ...item }) => item)
    : [];
  const profile = {
    id: user.id,
    email: user.email,
    display_name: user.display_name,
    plus: user.plus === true,
  };
  return [profile, ...previous.filter((item) => item?.id !== profile.id)].slice(
    0,
    8,
  );
}

export function dropAccountToken(list, userId) {
  if (!Array.isArray(list)) return [];
  return list.map((item) => {
    if (item?.id !== userId) return item;
    const profile = { ...item };
    delete profile.token;
    return profile;
  });
}
