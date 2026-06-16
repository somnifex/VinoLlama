import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, test, vi } from "vitest";
import App from "./App";

const apiBase = "http://127.0.0.1:11435";

function setupFetch() {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    calls.push({ url, init });

    if (url === `${apiBase}/api/version`) {
      return jsonResponse({ name: "VinoLlama", version: "test" });
    }
    if (url === `${apiBase}/api/runtime`) {
      return jsonResponse({ backend: "auto", processes: [] });
    }
    if (url === `${apiBase}/api/tags`) {
      return jsonResponse({ models: [{ name: "demo-model", parameters: "7B", quantization: "Q4_K_M" }] });
    }
    if (url === `${apiBase}/api/settings`) {
      return jsonResponse({
        server: { host: "127.0.0.1", port: 11435 },
        runtime: { backend: "auto", idle_timeout: "10m0s" },
        generation: { ctx_size: 4096, temperature: 0.7, top_p: 0.9, threads: 0 },
        privacy: { telemetry: false },
      });
    }
    if (url === `${apiBase}/api/doctor`) {
      return jsonResponse([]);
    }
    if (url === `${apiBase}/api/logs?limit=160`) {
      return jsonResponse({ lines: [] });
    }
    if (url === `${apiBase}/api/conversations`) {
      return jsonResponse({
        conversations: [
          {
            id: "conv-1",
            title: "Saved local chat",
            model: "demo-model",
            created_at: "2026-06-16T01:00:00Z",
            updated_at: "2026-06-16T01:05:00Z",
            message_count: 2,
          },
        ],
      });
    }
    if (url === `${apiBase}/api/conversations/conv-1`) {
      return jsonResponse({
        id: "conv-1",
        title: "Saved local chat",
        model: "demo-model",
        created_at: "2026-06-16T01:00:00Z",
        updated_at: "2026-06-16T01:05:00Z",
        messages: [
          { role: "system", content: "Answer briefly." },
          { role: "user", content: "hello" },
          { role: "assistant", content: "Hi there." },
        ],
      });
    }
    if (url === `${apiBase}/api/chat`) {
      return streamResponse([
        JSON.stringify({ message: { role: "assistant", content: "Local answer." }, done: false }),
        JSON.stringify({ done: true }),
      ]);
    }
    if (url === `${apiBase}/api/settings` && init?.method === "POST") {
      return jsonResponse({ restart_required: false });
    }

    return jsonResponse({}, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, calls };
}

describe("App chat experience", () => {
  test("renders the simple chat shell and toggles theme and settings panel", async () => {
    setupFetch();
    render(<App />);

    expect(await screen.findByText("New chat")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "VinoLlama logo" })).toBeInTheDocument();
    expect(screen.getByLabelText("Chat settings")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Hide panel" }));
    expect(screen.getByLabelText("Chat settings")).not.toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Switch to dark mode" }));
    expect(screen.getByRole("main")).toHaveAttribute("data-theme", "dark");
  });

  test("sends system prompt and user message without the welcome text", async () => {
    const { calls } = setupFetch();
    render(<App />);

    await screen.findByText("New chat");
    await userEvent.type(screen.getByPlaceholderText("Optional instructions for this conversation"), "Keep it short.");
    await userEvent.type(screen.getByLabelText("Message"), "What can you do?");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await screen.findByText("Local answer.");
    const chatCall = calls.find((call) => call.url === `${apiBase}/api/chat`);
    expect(chatCall).toBeDefined();
    const body = JSON.parse(String(chatCall?.init?.body));
    expect(body.model).toBe("demo-model");
    expect(body.messages).toEqual([
      { role: "system", content: "Keep it short." },
      { role: "user", content: "What can you do?" },
    ]);
  });

  test("opens a saved local conversation and restores its system prompt", async () => {
    setupFetch();
    render(<App />);

    const conversationButton = (await screen.findByText("Saved local chat")).closest("button");
    if (!conversationButton) {
      throw new Error("Saved conversation button was not found.");
    }
    await userEvent.click(conversationButton);

    expect(await screen.findByText("Hi there.")).toBeInTheDocument();
    const settingsPanel = screen.getByLabelText("Chat settings");
    expect(within(settingsPanel).getByDisplayValue("Answer briefly.")).toBeInTheDocument();
  });
});

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function streamResponse(lines: string[]) {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(`${lines.join("\n")}\n`));
        controller.close();
      },
    }),
    { status: 200 },
  );
}
