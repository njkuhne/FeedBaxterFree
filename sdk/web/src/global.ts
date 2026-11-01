import { createClient, type Client, type Options, type SendInput, type SendResult } from "./client.js";

let current: Client | undefined;

export { createClient };

export function init(opts: Options): Client {
  current = createClient(opts);
  return current;
}

export function send(input: SendInput): Promise<SendResult> {
  if (!current) throw new Error("FeedBaxter: call FeedBaxter.init({ url, key }) before send().");
  return current.send(input);
}
