export type ServiceStatus = {
  running: boolean;
  base_url: string;
  name?: string;
  version?: string;
  error?: string;
};

export type RuntimeProcess = {
  model: string;
  backend: string;
  pid: number;
  port: number;
  state: string;
  started_at?: string;
  last_used_at?: string;
  log_path?: string;
};

export type RuntimeStatus = {
  backend: string;
  idle_timeout?: string;
  processes: RuntimeProcess[];
};

export type ModelInfo = {
  name: string;
  model?: string;
  size?: number;
  modified_at?: string;
  parameters?: string;
  quantization?: string;
  backend_hint?: string;
};

export type SettingsStatus = {
  server?: {
    host?: string;
    port?: number;
  };
  runtime?: {
    backend?: string;
    idle_timeout?: string;
    ready_timeout?: string;
    llama_openvino_bin?: string;
    llama_cpu_bin?: string;
  };
  generation?: {
    ctx_size?: number;
    temperature?: number;
    top_p?: number;
    threads?: number;
  };
  privacy?: {
    telemetry?: boolean;
  };
  persisted?: boolean;
  restart_required?: boolean;
};

export type DoctorCheck = {
  level: "PASS" | "WARN" | "FAIL" | string;
  name: string;
  what?: string;
  reason?: string;
  fix?: string;
  details?: string;
};

export type LogsResponse = {
  logs?: LogEntry[];
  lines?: string[];
  log_dir?: string;
  error?: string;
};

export type LogEntry = {
  file: string;
  modified_at?: string;
  lines: string[];
};

const baseURL = "http://127.0.0.1:11435";

export type ChatMessage = {
  role: "system" | "user" | "assistant";
  content: string;
};

export type ChatStreamChunk = {
  model?: string;
  message?: ChatMessage;
  done?: boolean;
  error?: string;
};

export type ImportModelRequest = {
  name: string;
  path: string;
  mode: "reference" | "copy" | "link";
};

export type Conversation = {
  id: string;
  title: string;
  model: string;
  created_at: string;
  updated_at: string;
  messages: ChatMessage[];
};

export type ConversationSummary = {
  id: string;
  title: string;
  model: string;
  created_at: string;
  updated_at: string;
  message_count?: number;
};

export type ConversationExport = {
  id: string;
  format: "markdown" | string;
  content: string;
};

export async function fetchServiceStatus(signal?: AbortSignal): Promise<ServiceStatus> {
  try {
    const response = await fetch(`${baseURL}/api/version`, { signal });
    if (!response.ok) {
      return { running: false, base_url: baseURL, error: `HTTP ${response.status}` };
    }
    const data = (await response.json()) as { name?: string; version?: string };
    return {
      running: true,
      base_url: baseURL,
      name: data.name ?? "VinoLlama",
      version: data.version ?? "unknown",
    };
  } catch (error) {
    return {
      running: false,
      base_url: baseURL,
      error: error instanceof Error ? error.message : "local service is not running",
    };
  }
}

export async function fetchRuntimeStatus(signal?: AbortSignal): Promise<RuntimeStatus | null> {
  return fetchJSON<RuntimeStatus>("/api/runtime", signal);
}

export async function fetchModels(signal?: AbortSignal): Promise<ModelInfo[]> {
  const payload = await fetchJSON<{ models?: ModelInfo[] }>("/api/tags", signal);
  return payload?.models ?? [];
}

export async function fetchSettings(signal?: AbortSignal): Promise<SettingsStatus | null> {
  return fetchJSON<SettingsStatus>("/api/settings", signal);
}

export async function saveSettings(patch: SettingsStatus): Promise<SettingsStatus> {
  return postJSON<SettingsStatus>("/api/settings", patch);
}

export async function fetchDoctor(signal?: AbortSignal): Promise<DoctorCheck[]> {
  const payload = await fetchJSON<DoctorCheck[] | { checks?: DoctorCheck[]; report?: DoctorCheck[] }>("/api/doctor", signal);
  if (Array.isArray(payload)) {
    return payload;
  }
  return payload?.checks ?? payload?.report ?? [];
}

export async function fetchLogs(signal?: AbortSignal): Promise<LogsResponse | null> {
  return fetchJSON<LogsResponse>("/api/logs?limit=160", signal);
}

export async function fetchConversations(signal?: AbortSignal): Promise<ConversationSummary[]> {
  const payload = await fetchJSON<{ conversations?: ConversationSummary[] }>("/api/conversations", signal);
  return payload?.conversations ?? [];
}

export async function fetchConversation(id: string, signal?: AbortSignal): Promise<Conversation | null> {
  return fetchJSON<Conversation>(`/api/conversations/${encodeURIComponent(id)}`, signal);
}

export async function importModel(request: ImportModelRequest): Promise<ModelInfo> {
  return postJSON<ModelInfo>("/api/models/import", request);
}

export async function stopRuntimeModel(model: string): Promise<void> {
  await postJSON<{ stopped: boolean }>("/api/runtime/stop", { model });
}

export async function restartRuntimeModel(model: string, backend?: string): Promise<void> {
  await postJSON<{ restarted: boolean }>("/api/runtime/restart", { model, backend });
}

export async function saveConversation({
  title,
  model,
  messages,
}: {
  title: string;
  model: string;
  messages: ChatMessage[];
}): Promise<Conversation> {
  return postJSON<Conversation>("/api/conversations", { title, model, messages });
}

export async function updateConversation(
  id: string,
  patch: Partial<Pick<Conversation, "title" | "model" | "messages">>,
): Promise<Conversation> {
  return putJSON<Conversation>(`/api/conversations/${encodeURIComponent(id)}`, patch);
}

export async function deleteConversation(id: string): Promise<void> {
  await deleteJSON(`/api/conversations/${encodeURIComponent(id)}`);
}

export async function exportConversationMarkdown(id: string): Promise<ConversationExport> {
  return postJSON<ConversationExport>(`/api/conversations/${encodeURIComponent(id)}/export`, { format: "markdown" });
}

export async function sendChatStream({
  model,
  messages,
  signal,
  onChunk,
}: {
  model: string;
  messages: ChatMessage[];
  signal?: AbortSignal;
  onChunk: (content: string) => void;
}): Promise<string> {
  const response = await fetch(`${baseURL}/api/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    signal,
    body: JSON.stringify({ model, messages, stream: true }),
  });
  if (!response.ok) {
    const detail = await safeErrorDetail(response);
    throw new Error(detail || `Chat request failed with HTTP ${response.status}`);
  }
  if (!response.body) {
    throw new Error("Chat response body was empty.");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let reply = "";

  for (;;) {
    const { value, done } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      const chunk = parseStreamLine(line);
      if (!chunk) {
        continue;
      }
      if (chunk.error) {
        throw new Error(chunk.error);
      }
      const content = chunk.message?.content ?? "";
      if (content) {
        reply += content;
        onChunk(content);
      }
      if (chunk.done) {
        return reply;
      }
    }
  }

  if (buffer.trim()) {
    const chunk = parseStreamLine(buffer);
    const content = chunk?.message?.content ?? "";
    if (content) {
      reply += content;
      onChunk(content);
    }
  }
  return reply;
}

async function fetchJSON<T>(path: string, signal?: AbortSignal): Promise<T | null> {
  try {
    const response = await fetch(`${baseURL}${path}`, { signal });
    if (!response.ok) {
      return null;
    }
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

async function postJSON<T>(path: string, payload: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    signal,
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const detail = await safeErrorDetail(response);
    throw new Error(detail || `Request failed with HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

async function putJSON<T>(path: string, payload: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    signal,
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const detail = await safeErrorDetail(response);
    throw new Error(detail || `Request failed with HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

async function deleteJSON(path: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`${baseURL}${path}`, {
    method: "DELETE",
    signal,
  });
  if (!response.ok) {
    const detail = await safeErrorDetail(response);
    throw new Error(detail || `Request failed with HTTP ${response.status}`);
  }
}

function parseStreamLine(line: string): ChatStreamChunk | null {
  const trimmed = line.trim();
  if (!trimmed) {
    return null;
  }
  return JSON.parse(trimmed) as ChatStreamChunk;
}

async function safeErrorDetail(response: Response): Promise<string> {
  try {
    const data = (await response.json()) as { error?: string; reason?: string; what?: string; fix?: string };
    return [data.what, data.reason ?? data.error, data.fix].filter(Boolean).join(" ");
  } catch {
    return "";
  }
}
