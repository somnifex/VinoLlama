import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, test, vi } from "vitest";
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
    if (url === `${apiBase}/api/settings` && init?.method === "POST") {
      return jsonResponse({ restart_required: false });
    }
    if (url === `${apiBase}/api/settings`) {
      return jsonResponse({
        server: { host: "127.0.0.1", port: 11435 },
        runtime: {
          backend: "auto",
          idle_timeout: "10m0s",
          ready_timeout: "30s",
          llama_openvino_bin: "",
          llama_cpu_bin: "",
          openvino_device: "",
          internal_port_start: 21435,
          health_path: "",
          extra_openvino_args: [],
          extra_cpu_args: [],
          allow_unverified_flags: false,
        },
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
    return jsonResponse({}, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, calls };
}

describe("App chat experience", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.localStorage.setItem("vinollama.language", "en");
  });

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

  test("saves OpenVINO and llama.cpp backend settings from the GUI", async () => {
    const { calls } = setupFetch();
    render(<App />);

    await screen.findByText("New chat");
    await userEvent.click(screen.getByRole("button", { name: "Settings" }));
    await userEvent.selectOptions(screen.getByLabelText("Backend"), "openvino");
    await userEvent.type(screen.getByLabelText("OpenVINO llama.cpp server"), "C:\\tools\\llama-openvino.exe");
    await userEvent.type(screen.getByLabelText("CPU llama.cpp server"), "C:\\tools\\llama-cpu.exe");
    await userEvent.type(screen.getByLabelText("OpenVINO device"), "GPU.0");
    await userEvent.clear(screen.getByLabelText("OpenVINO extra args"));
    await userEvent.type(screen.getByLabelText("OpenVINO extra args"), "--device GPU");
    await userEvent.click(screen.getByLabelText("Allow unverified llama.cpp flags"));
    await userEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => {
      const settingsCall = calls.find((call) => call.url === `${apiBase}/api/settings` && call.init?.method === "POST");
      expect(settingsCall).toBeDefined();
      const body = JSON.parse(String(settingsCall?.init?.body));
      expect(body.runtime).toMatchObject({
        backend: "openvino",
        llama_openvino_bin: "C:\\tools\\llama-openvino.exe",
        llama_cpu_bin: "C:\\tools\\llama-cpu.exe",
        openvino_device: "GPU.0",
        extra_openvino_args: ["--device", "GPU"],
        allow_unverified_flags: true,
      });
    });
  });

  test("switches the desktop shell to Simplified Chinese", async () => {
    setupFetch();
    render(<App />);

    expect(await screen.findByText("New chat")).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("Language"), "zh-CN");

    expect(await screen.findByText("新聊天")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "设置" })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("lang", "zh-CN");
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
