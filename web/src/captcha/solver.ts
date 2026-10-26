// Client side of the bouncer handshake: request a challenge, solve it in a
// worker, exchange the answer for a short-lived pass cookie.

type Params = { alg: string; salt: string; degree: number };
type Challenge = { token: string; params: Params };

const MAX_DEGREE = 24;

async function requestChallenge(base: string): Promise<Challenge> {
  const response = await fetch(`${trimBase(base)}_bouncer/challenge`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: "{}",
    credentials: "same-origin",
  });
  if (!response.ok) throw new Error(`challenge failed: ${response.status}`);
  return (await response.json()) as Challenge;
}

function solve(params: Params): Promise<string> {
  if (params.alg !== "sha256") return Promise.reject(new Error("unsupported algorithm"));
  if (params.degree > MAX_DEGREE) return Promise.reject(new Error("difficulty above budget"));
  const salt = fromHex(params.salt);
  const worker = new Worker(new URL("./worker.ts", import.meta.url), { type: "module" });
  return new Promise((resolve, reject) => {
    worker.onmessage = (event: MessageEvent<{ answer: string }>) => {
      worker.terminate();
      resolve(event.data.answer);
    };
    worker.onerror = (error) => {
      worker.terminate();
      reject(error);
    };
    worker.postMessage({ salt, degree: params.degree });
  });
}

async function verify(token: string, answer: string, base: string): Promise<void> {
  const response = await fetch(`${trimBase(base)}_bouncer/verify`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ token, answer }),
    credentials: "same-origin",
  });
  if (!response.ok) throw new Error(`verification failed: ${response.status}`);
}

export async function solveChallenge(base = "/"): Promise<void> {
  const challenge = await requestChallenge(base);
  const answer = await solve(challenge.params);
  await verify(challenge.token, answer, base);
}

function trimBase(base: string): string {
  return base.endsWith("/") ? base : `${base}/`;
}

function fromHex(value: string): Uint8Array {
  const out = new Uint8Array(value.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = Number.parseInt(value.slice(i * 2, i * 2 + 2), 16);
  return out;
}
