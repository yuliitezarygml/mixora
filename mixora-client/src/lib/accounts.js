export function rememberAccount(list, user, token) {
  if (!user?.id) return Array.isArray(list) ? list.slice(0, 8) : [];
  const previous = Array.isArray(list) ? list : [];
  const existing = previous.find((item) => item?.id === user.id);
  const saved =
    typeof token === "string" && token.length > 0
      ? token
      : typeof existing?.token === "string"
        ? existing.token
        : "";
  const profile = {
    id: user.id,
    email: user.email,
    display_name: user.display_name,
    plus: user.plus === true,
    ...(saved ? { token: saved } : {}),
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
