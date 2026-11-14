export type State = "ready" | "preparing" | "error" | "unknown";
export type Health = {
  status: State;
  mode?: "demo" | "live";
  api: State;
  database: State;
  worker: State;
  checks?: number;
  requestId?: string;
};
export const unavailableHealth: Health = {
  status: "error",
  api: "error",
  database: "unknown",
  worker: "unknown",
};
