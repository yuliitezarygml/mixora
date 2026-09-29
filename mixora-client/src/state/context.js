import { createContext, useContext } from "react";
export const AppContext = createContext(null);
export function useApp() {
  const value = useContext(AppContext);
  if (!value) throw new Error("Mixora components must be inside AppProvider");
  return value;
}
