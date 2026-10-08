import settings from "../api.json" with { type: "json" };
import { createNativeTransport } from "./nativeTransport.js";

// One public mobile API entry point. Server addresses live only in api.json;
// source-aware endpoints/normalization remain shared with the PC client.
export const apiSettings = Object.freeze(settings);
export const createMobileAPI = (invoke) => createNativeTransport(invoke);
export * from "../../mixora-client/src/lib/api.js";
