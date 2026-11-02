import { CodeBlock } from "../components/CodeBlock";

interface McpConfigProps {
  /** The token just created. Shown once, with the config it belongs in. */
  secret: string;
  origin?: string;
}

export function claudeCodeCommand(origin: string, secret: string): string {
  return `claude mcp add --transport http feedbaxter ${origin}/mcp --header "Authorization: Bearer ${secret}"`;
}

export function jsonConfig(origin: string, secret: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        feedbaxter: {
          type: "http",
          url: `${origin}/mcp`,
          headers: { Authorization: `Bearer ${secret}` },
        },
      },
    },
    null,
    2,
  );
}

/** Ready-to-paste MCP client config for a token that was just created. */
export function McpConfig({ secret, origin = window.location.origin }: McpConfigProps) {
  return (
    <section className="mcp-config" aria-labelledby="mcp-config-h">
      <h3 id="mcp-config-h" className="secret-title">
        Connect your AI client
      </h3>
      <p className="soft">Paste one of these into your client. It includes the token, so keep it private.</p>
      <CodeBlock code={claudeCodeCommand(origin, secret)} label="Claude Code" />
      <CodeBlock code={jsonConfig(origin, secret)} label="JSON config (Claude Desktop and others)" />
    </section>
  );
}
