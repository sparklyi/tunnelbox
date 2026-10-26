import { leadingZeroBits, sha256, toHex } from "./sha256";

type SolveRequest = { salt: Uint8Array; degree: number };

type WorkerScope = {
  onmessage: ((event: MessageEvent<SolveRequest>) => void) | null;
  postMessage: (message: { answer: string }) => void;
};

const scope = self as unknown as WorkerScope;
const TAG = new TextEncoder().encode("bouncer/pow/v1");
const preimage = new Uint8Array(TAG.length + 16 + 8);
preimage.set(TAG, 0);

scope.onmessage = (event) => {
  preimage.set(event.data.salt, TAG.length);
  const view = new DataView(preimage.buffer);
  let nonce = 1 + Math.floor(Math.random() * 0xffff);

  for (;;) {
    view.setUint32(TAG.length + 16, Math.floor(nonce / 0x100000000));
    view.setUint32(TAG.length + 20, nonce >>> 0);
    if (leadingZeroBits(sha256(preimage)) >= event.data.degree) {
      const raw = new Uint8Array(8);
      const rawView = new DataView(raw.buffer);
      rawView.setUint32(0, Math.floor(nonce / 0x100000000));
      rawView.setUint32(4, nonce >>> 0);
      scope.postMessage({ answer: toHex(raw) });
      return;
    }
    nonce += 1;
  }
};
