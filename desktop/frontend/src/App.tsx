import { useCallback, useEffect, useMemo, useState, type FormEvent, type KeyboardEvent, type ReactNode } from "react";
import {
  ChatMessage,
  ConversationSummary,
  deleteConversation,
  DoctorCheck,
  exportConversationMarkdown,
  fetchConversation,
  fetchConversations,
  fetchDoctor,
  fetchLogs,
  fetchModels,
  fetchRuntimeStatus,
  fetchServiceStatus,
  fetchSettings,
  importModel,
  LogsResponse,
  ModelInfo,
  RuntimeStatus,
  ServiceStatus,
  SettingsStatus,
  restartRuntimeModel,
  saveConversation,
  saveSettings,
  sendChatStream,
  stopRuntimeModel,
  updateConversation,
} from "./api";

const navItems = ["Chat", "Models", "Runtime", "Settings", "Doctor", "Logs"] as const;
type NavItem = (typeof navItems)[number];
type ThemeMode = "light" | "dark";

const welcomeMessages: ChatMessage[] = [
  {
    role: "assistant",
    content:
      "VinoLlama is ready to talk to your local models. Start the API service, choose a GGUF model, then keep the whole session on this machine.",
  },
];

export default function App() {
  const [active, setActive] = useState<NavItem>("Chat");
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme());
  const [service, setService] = useState<ServiceStatus>({
    running: false,
    base_url: "http://127.0.0.1:11435",
  });
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [settings, setSettings] = useState<SettingsStatus | null>(null);
  const [doctor, setDoctor] = useState<DoctorCheck[]>([]);
  const [logs, setLogs] = useState<LogsResponse | null>(null);
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedModel, setSelectedModel] = useState("");

  const refresh = useCallback(async () => {
    const controller = new AbortController();
    setRefreshing(true);
    try {
      const [nextService, nextRuntime, nextModels, nextSettings, nextDoctor, nextLogs, nextConversations] = await Promise.all([
        fetchServiceStatus(controller.signal),
        fetchRuntimeStatus(controller.signal),
        fetchModels(controller.signal),
        fetchSettings(controller.signal),
        fetchDoctor(controller.signal),
        fetchLogs(controller.signal),
        fetchConversations(controller.signal),
      ]);
      setService(nextService);
      setRuntime(nextRuntime);
      setModels(nextModels);
      setSettings(nextSettings);
      setDoctor(nextDoctor);
      setLogs(nextLogs);
      setConversations(nextConversations);
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  useEffect(() => {
    if (models.length === 0) {
      setSelectedModel("");
      return;
    }
    setSelectedModel((current) => (current && models.some((model) => model.name === current) ? current : models[0].name));
  }, [models]);

  const runtimeLabel = useMemo(() => {
    if (!service.running) {
      return "offline";
    }
    return runtime?.backend || settings?.runtime?.backend || "auto";
  }, [runtime?.backend, service.running, settings?.runtime?.backend]);

  const activeModel = selectedModel || "No model selected";
  const toggleTheme = () => setTheme((current) => (current === "light" ? "dark" : "light"));

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem("vinollama.theme", theme);
  }, [theme]);

  return (
    <main className="app-shell" data-theme={theme}>
      <aside className="sidebar" aria-label="Primary">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            VL
          </span>
          <div>
            <strong>VinoLlama</strong>
            <span>local model studio</span>
          </div>
        </div>

        <div className="service-card">
          <div className="service-card-row">
            <span className={service.running ? "status-dot online" : "status-dot"} />
            <strong>{service.running ? "Connected" : "Offline"}</strong>
          </div>
          <span>{service.base_url}</span>
        </div>

        <nav className="nav-list">
          {navItems.map((item) => (
            <button
              key={item}
              className={item === active ? "nav-item active" : "nav-item"}
              type="button"
              onClick={() => setActive(item)}
            >
              <NavIcon name={item} />
              <span>{item}</span>
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <span>Default bind</span>
          <strong>127.0.0.1:11435</strong>
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div className="crumbs">
            <span>{active}</span>
            <strong>{active === "Chat" ? activeModel : "Local workspace"}</strong>
          </div>
          <div className="topbar-actions">
            <span className="runtime-pill">backend {runtimeLabel}</span>
            <span className="runtime-pill">{models.length} models</span>
            <button
              type="button"
              className="icon-button"
              onClick={toggleTheme}
              aria-label={`Switch to ${theme === "light" ? "dark" : "light"} mode`}
              title={`Switch to ${theme === "light" ? "dark" : "light"} mode`}
            >
              <ThemeIcon mode={theme} />
            </button>
            <button
              type="button"
              className="icon-button"
              onClick={() => void refresh()}
              aria-label="Refresh service status"
              title="Refresh service status"
            >
              <RefreshIcon spinning={refreshing} />
            </button>
          </div>
        </header>

        {active === "Chat" && (
          <ChatPanel
            service={service}
            models={models}
            runtime={runtime}
            settings={settings}
            conversations={conversations}
            selectedModel={selectedModel}
            onSelectModel={setSelectedModel}
            onRefresh={refresh}
          />
        )}
        {active === "Models" && (
          <ModelsPanel
            service={service}
            models={models}
            onUseModel={setSelectedModel}
            onNavigate={setActive}
            onRefresh={refresh}
          />
        )}
        {active === "Runtime" && <RuntimePanel service={service} runtime={runtime} settings={settings} onRefresh={refresh} />}
        {active === "Settings" && <SettingsPanel service={service} settings={settings} onRefresh={refresh} />}
        {active === "Doctor" && <DoctorPanel service={service} checks={doctor} />}
        {active === "Logs" && <LogsPanel service={service} logs={logs} />}
      </section>
    </main>
  );
}

function ChatPanel({
  service,
  models,
  runtime,
  settings,
  conversations,
  selectedModel,
  onSelectModel,
  onRefresh,
}: {
  service: ServiceStatus;
  models: ModelInfo[];
  runtime: RuntimeStatus | null;
  settings: SettingsStatus | null;
  conversations: ConversationSummary[];
  selectedModel: string;
  onSelectModel: (model: string) => void;
  onRefresh: () => Promise<void>;
}) {
  const [messages, setMessages] = useState<ChatMessage[]>(welcomeMessages);
  const [draft, setDraft] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState("");
  const [saveNotice, setSaveNotice] = useState("");
  const [controller, setController] = useState<AbortController | null>(null);
  const [activeConversationId, setActiveConversationId] = useState("");
  const [busyConversation, setBusyConversation] = useState("");
  const [inspectorOpen, setInspectorOpen] = useState(true);
  const [systemPrompt, setSystemPrompt] = useState("");
  const [chatBackend, setChatBackend] = useState(settings?.runtime?.backend ?? "auto");
  const [chatCtxSize, setChatCtxSize] = useState(`${settings?.generation?.ctx_size ?? 4096}`);
  const [chatTemperature, setChatTemperature] = useState(`${settings?.generation?.temperature ?? 0.7}`);
  const [chatTopP, setChatTopP] = useState(`${settings?.generation?.top_p ?? 0.9}`);
  const [chatThreads, setChatThreads] = useState(`${settings?.generation?.threads ?? 0}`);
  const [savingChatSettings, setSavingChatSettings] = useState(false);
  const [chatSettingsNotice, setChatSettingsNotice] = useState("");

  const canSend = service.running && selectedModel !== "" && draft.trim() !== "" && !isStreaming;
  const canSave = service.running && selectedModel !== "" && messages.some((message) => message.role === "user" && message.content.trim() !== "");
  const visibleModel = selectedModel || "Select model";

  useEffect(() => {
    setChatBackend(settings?.runtime?.backend ?? "auto");
    setChatCtxSize(`${settings?.generation?.ctx_size ?? 4096}`);
    setChatTemperature(`${settings?.generation?.temperature ?? 0.7}`);
    setChatTopP(`${settings?.generation?.top_p ?? 0.9}`);
    setChatThreads(`${settings?.generation?.threads ?? 0}`);
  }, [settings]);

  const sendMessage = async () => {
    const content = draft.trim();
    if (!canSend || !content) {
      return;
    }
    const aborter = new AbortController();
    const nextMessages: ChatMessage[] = [...messages, { role: "user", content }, { role: "assistant", content: "" }];
    setMessages(nextMessages);
    setDraft("");
    setError("");
    setIsStreaming(true);
    setController(aborter);

    try {
      const requestMessages = buildPromptMessages(nextMessages, systemPrompt);
      await sendChatStream({
        model: selectedModel,
        messages: requestMessages,
        signal: aborter.signal,
        onChunk: (chunk) => {
          setMessages((current) => {
            const copy = [...current];
            const last = copy[copy.length - 1];
            if (last?.role === "assistant") {
              copy[copy.length - 1] = { ...last, content: last.content + chunk };
            }
            return copy;
          });
        },
      });
    } catch (err) {
      if (aborter.signal.aborted) {
        setError("Generation stopped.");
      } else {
        setError(err instanceof Error ? err.message : "Chat request failed.");
      }
      setMessages((current) => current.filter((message, index) => index !== current.length - 1 || message.content !== ""));
    } finally {
      setIsStreaming(false);
      setController(null);
    }
  };

  const stopMessage = () => {
    controller?.abort();
  };

  const startNewConversation = () => {
    controller?.abort();
    setMessages(welcomeMessages);
    setDraft("");
    setError("");
    setSaveNotice("");
    setActiveConversationId("");
    setSystemPrompt("");
  };

  const openConversation = async (id: string) => {
    if (!service.running || isStreaming) {
      return;
    }
    setBusyConversation(`open:${id}`);
    setSaveNotice("");
    setError("");
    try {
      const conversation = await fetchConversation(id);
      if (!conversation) {
        setSaveNotice("Conversation could not be loaded.");
        return;
      }
      const split = splitConversationMessages(conversation.messages);
      setSystemPrompt(split.systemPrompt);
      setMessages(split.chatMessages.length > 0 ? split.chatMessages : welcomeMessages);
      setActiveConversationId(conversation.id);
      if (conversation.model) {
        onSelectModel(conversation.model);
      }
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : "Conversation could not be loaded.");
    } finally {
      setBusyConversation("");
    }
  };

  const saveCurrentConversation = async () => {
    const contentMessages = buildStoredMessages(messages, systemPrompt);
    if (!canSave || contentMessages.length === 0) {
      return;
    }
    setBusyConversation("save");
    setSaveNotice("");
    try {
      const title = getConversationTitle(contentMessages);
      const conversation = activeConversationId
        ? await updateConversation(activeConversationId, { title, model: selectedModel, messages: contentMessages })
        : await saveConversation({ title, model: selectedModel, messages: contentMessages });
      setActiveConversationId(conversation.id);
      setSaveNotice(`Saved ${conversation.title}.`);
      await onRefresh();
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : "Conversation could not be saved.");
    } finally {
      setBusyConversation("");
    }
  };

  const saveChatSettings = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!service.running) {
      return;
    }
    setSavingChatSettings(true);
    setChatSettingsNotice("");
    try {
      const ctxSize = Number(chatCtxSize);
      const temperature = Number(chatTemperature);
      const topP = Number(chatTopP);
      const threads = Number(chatThreads);
      if (!Number.isFinite(ctxSize) || ctxSize < 512 || !Number.isFinite(temperature) || temperature < 0 || temperature > 2 || !Number.isFinite(topP) || topP < 0 || topP > 1 || !Number.isFinite(threads) || threads < 0) {
        setChatSettingsNotice("Check context, temperature, Top P, and threads before saving.");
        return;
      }
      const saved = await saveSettings({
        runtime: {
          backend: chatBackend,
        },
        generation: {
          ctx_size: ctxSize,
          temperature,
          top_p: topP,
          threads,
        },
        privacy: {
          telemetry: false,
        },
      });
      setChatSettingsNotice(saved.restart_required ? "Saved. Restart may be required." : "Settings saved.");
      await onRefresh();
    } catch (err) {
      setChatSettingsNotice(err instanceof Error ? err.message : "Settings could not be saved.");
    } finally {
      setSavingChatSettings(false);
    }
  };

  const deleteStoredConversation = async (id: string, title: string) => {
    if (!service.running || isStreaming || !window.confirm(`Delete "${title}" from local conversations?`)) {
      return;
    }
    setBusyConversation(`delete:${id}`);
    setSaveNotice("");
    try {
      await deleteConversation(id);
      if (id === activeConversationId) {
        startNewConversation();
      }
      setSaveNotice("Conversation deleted.");
      await onRefresh();
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : "Conversation could not be deleted.");
    } finally {
      setBusyConversation("");
    }
  };

  const exportStoredConversation = async (id: string) => {
    if (!service.running) {
      return;
    }
    setBusyConversation(`export:${id}`);
    setSaveNotice("");
    try {
      const exported = await exportConversationMarkdown(id);
      await navigator.clipboard.writeText(exported.content);
      setSaveNotice("Conversation markdown copied.");
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : "Conversation could not be exported.");
    } finally {
      setBusyConversation("");
    }
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void sendMessage();
  };

  const handleComposerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  };

  return (
    <section className={inspectorOpen ? "chat-layout" : "chat-layout inspector-collapsed"} aria-label="Chat workspace">
      <div className="conversation-rail">
        <button type="button" className="primary-action" disabled={isStreaming} onClick={startNewConversation}>
          <PlusIcon />
          <span>New chat</span>
        </button>
        <div className="rail-section">
          <span className="rail-label">Recent</span>
          {conversations.length === 0 ? (
            <span className="rail-empty">No saved conversations yet.</span>
          ) : (
            conversations.map((conversation) => (
              <div className="conversation-row" key={conversation.id}>
                <button
                  type="button"
                  className={conversation.id === activeConversationId ? "conversation-item active" : "conversation-item"}
                  disabled={!service.running || isStreaming || busyConversation !== ""}
                  onClick={() => void openConversation(conversation.id)}
                >
                  <strong>{conversation.title || "Untitled conversation"}</strong>
                  <span>{conversation.model || "unknown model"}</span>
                  <small>{formatRelativeDate(conversation.updated_at)}</small>
                </button>
                <div className="conversation-actions" aria-label={`Actions for ${conversation.title}`}>
                  <button
                    type="button"
                    className="mini-action"
                    disabled={!service.running || busyConversation !== ""}
                    onClick={() => void exportStoredConversation(conversation.id)}
                    title="Copy markdown export"
                    aria-label={`Copy markdown export for ${conversation.title}`}
                  >
                    MD
                  </button>
                  <button
                    type="button"
                    className="mini-action danger-action"
                    disabled={!service.running || isStreaming || busyConversation !== ""}
                    onClick={() => void deleteStoredConversation(conversation.id, conversation.title || "Untitled conversation")}
                    title="Delete conversation"
                    aria-label={`Delete ${conversation.title}`}
                  >
                    Del
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      <div className="chat-main">
        <div className="chat-toolbar">
          <label>
            <span>Model</span>
            <select
              aria-label="Model"
              disabled={!service.running || models.length === 0}
              value={visibleModel}
              onChange={(event) => onSelectModel(event.target.value)}
            >
              {models.length === 0 ? (
                <option>{visibleModel}</option>
              ) : (
                models.map((model) => (
                  <option key={model.name} value={model.name}>
                    {model.name}
                  </option>
                ))
              )}
            </select>
          </label>
          <div className="toolbar-cluster">
            <span className="service-chip">{service.running ? "API online" : "API offline"}</span>
            <span className="service-chip">{runtime?.processes.length ?? 0} running</span>
            <button
              type="button"
              className="secondary-action compact-action"
              onClick={() => setInspectorOpen((current) => !current)}
              aria-expanded={inspectorOpen}
              aria-controls="chat-settings-panel"
            >
              {inspectorOpen ? "Hide panel" : "Show panel"}
            </button>
            <button
              type="button"
              className="secondary-action compact-action"
              disabled={!canSave || busyConversation === "save"}
              onClick={() => void saveCurrentConversation()}
            >
              {busyConversation === "save" ? "Saving" : activeConversationId ? "Save" : "Save new"}
            </button>
          </div>
        </div>

        <div className="message-surface">
          {messages.map((turn, index) => (
            <article className={`message ${turn.role}`} key={`${turn.role}-${index}`}>
              <span>{turn.role}</span>
              <p>{turn.content || (isStreaming && index === messages.length - 1 ? "Thinking..." : "")}</p>
            </article>
          ))}
          {!service.running && (
            <div className="inline-banner">
              <strong>Start the local API</strong>
              <span>{service.error ?? "Run vinollama serve to connect the desktop shell."}</span>
            </div>
          )}
          {error && (
            <div className="inline-banner warning" role="status">
              <strong>{error}</strong>
              <span>{error === "Generation stopped." ? "Your local request was cancelled." : "Check runtime logs or run doctor."}</span>
            </div>
          )}
          {saveNotice && (
            <div className="inline-banner" role="status">
              <strong>{saveNotice}</strong>
            </div>
          )}
        </div>

        <form className="composer" onSubmit={handleSubmit}>
          <textarea
            aria-label="Message"
            disabled={!service.running || isStreaming}
            placeholder="Ask a local model..."
            rows={3}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={handleComposerKeyDown}
          />
          <div className="composer-actions">
            <button type="button" className="secondary-action" disabled={!isStreaming} onClick={stopMessage}>
              Stop
            </button>
            <button type="submit" disabled={!canSend}>
              {isStreaming ? "Sending" : "Send"}
            </button>
          </div>
        </form>
      </div>

      <aside className="inspector" id="chat-settings-panel" aria-label="Chat settings" hidden={!inspectorOpen}>
        <div className="inspector-header">
          <div>
            <span>Local settings</span>
            <strong>Chat controls</strong>
          </div>
          <button
            type="button"
            className="icon-button"
            onClick={() => setInspectorOpen(false)}
            aria-label="Hide chat settings"
            title="Hide chat settings"
          >
            <CloseIcon />
          </button>
        </div>
        <form className="inspector-form" onSubmit={saveChatSettings}>
          <label>
            <span>Backend</span>
            <select value={chatBackend} onChange={(event) => setChatBackend(event.target.value)} disabled={!service.running || savingChatSettings}>
              <option value="auto">auto</option>
              <option value="openvino">openvino</option>
              <option value="cpu">cpu</option>
            </select>
          </label>
          <label>
            <span>Context</span>
            <input
              type="number"
              min="512"
              step="512"
              value={chatCtxSize}
              onChange={(event) => setChatCtxSize(event.target.value)}
              disabled={!service.running || savingChatSettings}
            />
          </label>
          <label>
            <span>Temperature</span>
            <input
              type="number"
              min="0"
              max="2"
              step="0.1"
              value={chatTemperature}
              onChange={(event) => setChatTemperature(event.target.value)}
              disabled={!service.running || savingChatSettings}
            />
          </label>
          <label>
            <span>Top P</span>
            <input
              type="number"
              min="0"
              max="1"
              step="0.05"
              value={chatTopP}
              onChange={(event) => setChatTopP(event.target.value)}
              disabled={!service.running || savingChatSettings}
            />
          </label>
          <label>
            <span>Threads</span>
            <input
              type="number"
              min="0"
              step="1"
              value={chatThreads}
              onChange={(event) => setChatThreads(event.target.value)}
              disabled={!service.running || savingChatSettings}
            />
          </label>
          <label className="wide-field">
            <span>System prompt</span>
            <textarea
              value={systemPrompt}
              onChange={(event) => setSystemPrompt(event.target.value)}
              placeholder="Optional instructions for this conversation"
              rows={6}
            />
          </label>
          <button type="submit" disabled={!service.running || savingChatSettings}>
            {savingChatSettings ? "Saving" : "Save settings"}
          </button>
          {chatSettingsNotice && <span className="form-note">{chatSettingsNotice}</span>}
        </form>
        <div className="inspector-note">
          <strong>Privacy</strong>
          <span>Prompts and conversations stay local by default.</span>
        </div>
      </aside>
    </section>
  );
}

function ModelsPanel({
  service,
  models,
  onUseModel,
  onNavigate,
  onRefresh,
}: {
  service: ServiceStatus;
  models: ModelInfo[];
  onUseModel: (model: string) => void;
  onNavigate: (nav: NavItem) => void;
  onRefresh: () => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [mode, setMode] = useState<"reference" | "copy" | "link">("reference");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const handleImport = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!service.running || !name.trim() || !path.trim()) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await importModel({ name: name.trim(), path: path.trim(), mode });
      setNotice(`Imported ${name.trim()}.`);
      setName("");
      setPath("");
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Model import failed.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title="Models" kicker={service.running ? "Local model library" : "Waiting for API"} />
      <form className="import-strip" onSubmit={handleImport}>
        <label>
          <span>Name</span>
          <input value={name} onChange={(event) => setName(event.target.value)} disabled={!service.running || busy} />
        </label>
        <label className="path-field">
          <span>GGUF path</span>
          <input value={path} onChange={(event) => setPath(event.target.value)} disabled={!service.running || busy} />
        </label>
        <label>
          <span>Mode</span>
          <select value={mode} onChange={(event) => setMode(event.target.value as "reference" | "copy" | "link")} disabled={!service.running || busy}>
            <option value="reference">reference</option>
            <option value="copy">copy</option>
            <option value="link">link</option>
          </select>
        </label>
        <button type="submit" disabled={!service.running || busy || !name.trim() || !path.trim()}>
          {busy ? "Importing" : "Import"}
        </button>
      </form>
      {notice && (
        <div className="inline-banner warning" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <div className="model-grid">
        {models.length === 0 ? (
          <EmptyState title="No models imported" body="Import a local GGUF model from the CLI or API to make it appear here." />
        ) : (
          models.map((model) => (
            <article className="model-card" key={model.name}>
              <div>
                <strong>{model.name}</strong>
                <span>{model.parameters ?? "unknown"} parameters</span>
              </div>
              <dl>
                <MetricRow label="Quant" value={model.quantization ?? "unknown"} />
                <MetricRow label="Backend" value={model.backend_hint ?? "auto"} />
                <MetricRow label="Size" value={formatBytes(model.size)} />
              </dl>
              <button
                type="button"
                onClick={() => {
                  onUseModel(model.name);
                  onNavigate("Chat");
                }}
              >
                Use in chat
              </button>
            </article>
          ))
        )}
      </div>
    </section>
  );
}

function RuntimePanel({
  service,
  runtime,
  settings,
  onRefresh,
}: {
  service: ServiceStatus;
  runtime: RuntimeStatus | null;
  settings: SettingsStatus | null;
  onRefresh: () => Promise<void>;
}) {
  const [busyModel, setBusyModel] = useState("");
  const [notice, setNotice] = useState("");
  const runRuntimeAction = async (model: string, action: "stop" | "restart") => {
    setBusyModel(`${action}:${model}`);
    setNotice("");
    try {
      if (action === "stop") {
        await stopRuntimeModel(model);
      } else {
        await restartRuntimeModel(model, settings?.runtime?.backend);
      }
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : `${action} failed.`);
    } finally {
      setBusyModel("");
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title="Runtime" kicker={service.running ? "Process manager" : "Runtime offline"} />
      <div className="runtime-summary">
        <Metric label="Mode" value={runtime?.backend ?? settings?.runtime?.backend ?? "auto"} />
        <Metric label="Idle timeout" value={settings?.runtime?.idle_timeout ?? "10m"} />
        <Metric label="Processes" value={`${runtime?.processes.length ?? 0}`} />
      </div>
      <div className="process-table" role="table" aria-label="Runtime processes">
        <div className="table-row header" role="row">
          <span>Model</span>
          <span>Backend</span>
          <span>PID</span>
          <span>Port</span>
          <span>State</span>
          <span>Actions</span>
        </div>
        {(runtime?.processes ?? []).length === 0 ? (
          <div className="table-empty">No running models.</div>
        ) : (
          runtime?.processes.map((process) => (
            <div className="table-row" role="row" key={`${process.model}-${process.backend}-${process.pid}`}>
              <span>{process.model}</span>
              <span>{process.backend}</span>
              <span>{process.pid}</span>
              <span>{process.port}</span>
              <span>{process.state}</span>
              <span className="row-actions">
                <button type="button" onClick={() => void runRuntimeAction(process.model, "stop")} disabled={busyModel !== ""}>
                  {busyModel === `stop:${process.model}` ? "Stopping" : "Stop"}
                </button>
                <button type="button" onClick={() => void runRuntimeAction(process.model, "restart")} disabled={busyModel !== ""}>
                  {busyModel === `restart:${process.model}` ? "Restarting" : "Restart"}
                </button>
              </span>
            </div>
          ))
        )}
      </div>
      {notice && (
        <div className="inline-banner warning" role="status">
          <strong>{notice}</strong>
        </div>
      )}
    </section>
  );
}

function SettingsPanel({
  service,
  settings,
  onRefresh,
}: {
  service: ServiceStatus;
  settings: SettingsStatus | null;
  onRefresh: () => Promise<void>;
}) {
  const [backend, setBackend] = useState(settings?.runtime?.backend ?? "auto");
  const [idleTimeout, setIdleTimeout] = useState(settings?.runtime?.idle_timeout ?? "10m0s");
  const [ctxSize, setCtxSize] = useState(`${settings?.generation?.ctx_size ?? 4096}`);
  const [temperature, setTemperature] = useState(`${settings?.generation?.temperature ?? 0.7}`);
  const [topP, setTopP] = useState(`${settings?.generation?.top_p ?? 0.9}`);
  const [threads, setThreads] = useState(`${settings?.generation?.threads ?? 0}`);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setBackend(settings?.runtime?.backend ?? "auto");
    setIdleTimeout(settings?.runtime?.idle_timeout ?? "10m0s");
    setCtxSize(`${settings?.generation?.ctx_size ?? 4096}`);
    setTemperature(`${settings?.generation?.temperature ?? 0.7}`);
    setTopP(`${settings?.generation?.top_p ?? 0.9}`);
    setThreads(`${settings?.generation?.threads ?? 0}`);
  }, [settings]);

  const submitSettings = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSaving(true);
    setNotice("");
    try {
      const saved = await saveSettings({
        runtime: {
          backend,
          idle_timeout: idleTimeout,
        },
        generation: {
          ctx_size: Number(ctxSize),
          temperature: Number(temperature),
          top_p: Number(topP),
          threads: Number(threads),
        },
        privacy: {
          telemetry: false,
        },
      });
      setNotice(saved.restart_required ? "Saved in memory. Restart may be required." : "Settings saved.");
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Settings could not be saved.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title="Settings" kicker={service.running ? "Active configuration" : "Read-only preview"} />
      <form className="settings-grid" onSubmit={submitSettings}>
        <SettingGroup title="Server">
          <MetricRow label="Host" value={settings?.server?.host ?? "127.0.0.1"} />
          <MetricRow label="Port" value={`${settings?.server?.port ?? 11435}`} />
        </SettingGroup>
        <SettingGroup title="Generation">
          <EditableRow label="Context" value={ctxSize} onChange={setCtxSize} disabled={!service.running || saving} />
          <EditableRow label="Temperature" value={temperature} onChange={setTemperature} disabled={!service.running || saving} />
          <EditableRow label="Top P" value={topP} onChange={setTopP} disabled={!service.running || saving} />
          <EditableRow label="Threads" value={threads} onChange={setThreads} disabled={!service.running || saving} />
        </SettingGroup>
        <SettingGroup title="Runtime">
          <label className="editable-row">
            <span>Backend</span>
            <select value={backend} onChange={(event) => setBackend(event.target.value)} disabled={!service.running || saving}>
              <option value="auto">auto</option>
              <option value="openvino">openvino</option>
              <option value="cpu">cpu</option>
            </select>
          </label>
          <EditableRow label="Idle timeout" value={idleTimeout} onChange={setIdleTimeout} disabled={!service.running || saving} />
        </SettingGroup>
        <SettingGroup title="Privacy">
          <MetricRow label="Telemetry" value={settings?.privacy?.telemetry ? "enabled" : "disabled"} />
          <MetricRow label="Cloud sync" value="not implemented" />
        </SettingGroup>
        <div className="settings-actions">
          <button type="submit" disabled={!service.running || saving}>
            {saving ? "Saving" : "Save settings"}
          </button>
          {notice && <span>{notice}</span>}
        </div>
      </form>
    </section>
  );
}

function DoctorPanel({ service, checks }: { service: ServiceStatus; checks: DoctorCheck[] }) {
  const [notice, setNotice] = useState("");
  const copyReport = async () => {
    const report = checks
      .map((check) => `[${check.level}] ${check.name}\nReason: ${check.reason || check.what || ""}\nFix: ${check.fix || ""}`)
      .join("\n\n");
    try {
      await navigator.clipboard.writeText(report || "No diagnostic report available.");
      setNotice("Diagnostic report copied.");
    } catch {
      setNotice("Clipboard access is unavailable.");
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title="Doctor" kicker={service.running ? "Diagnostics" : "Start API for live checks"}>
        <button type="button" className="secondary-action compact-action" onClick={() => void copyReport()}>
          Copy report
        </button>
      </PageHeader>
      {notice && (
        <div className="inline-banner" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <div className="doctor-list">
        {checks.length === 0 ? (
          <EmptyState title="No diagnostic report" body="Run the local service to view structured doctor checks here." />
        ) : (
          checks.map((check) => (
            <article className="doctor-row" key={`${check.name}-${check.level}`}>
              <span className={`level ${check.level.toLowerCase()}`}>{check.level}</span>
              <div>
                <strong>{check.name}</strong>
                <p>{check.reason || check.what || "Check completed."}</p>
                {check.fix && <small>{check.fix}</small>}
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}

function LogsPanel({ service, logs }: { service: ServiceStatus; logs: LogsResponse | null }) {
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState("");
  const lines = flattenLogs(logs);
  const filtered = query.trim()
    ? lines.filter((line) => line.toLowerCase().includes(query.trim().toLowerCase()))
    : lines;
  const copyLogs = async () => {
    try {
      await navigator.clipboard.writeText(filtered.join("\n"));
      setNotice("Log lines copied.");
    } catch {
      setNotice("Clipboard access is unavailable.");
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title="Logs" kicker={service.running ? logs?.log_dir ?? "Runtime log tail" : "Offline"}>
        <div className="toolbar-cluster">
          <input aria-label="Filter logs" placeholder="Filter logs" value={query} onChange={(event) => setQuery(event.target.value)} />
          <button type="button" className="secondary-action compact-action" onClick={() => void copyLogs()}>
            Copy
          </button>
        </div>
      </PageHeader>
      {notice && (
        <div className="inline-banner" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <pre className="log-view" aria-label="Runtime logs">
        {filtered.length === 0 ? "No log lines available." : filtered.join("\n")}
      </pre>
    </section>
  );
}

function PageHeader({ title, kicker, children }: { title: string; kicker: string; children?: ReactNode }) {
  return (
    <header className="page-header">
      <div>
        <span>{kicker}</span>
        <h1>{title}</h1>
      </div>
      {children}
    </header>
  );
}

function SettingGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="setting-group">
      <h2>{title}</h2>
      <dl>{children}</dl>
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function MetricRow({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </>
  );
}

function EditableRow({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <label className="editable-row">
      <span>{label}</span>
      <input value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled} />
    </label>
  );
}

function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      <span>{body}</span>
    </div>
  );
}

function NavIcon({ name }: { name: NavItem }) {
  const path = {
    Chat: "M5 6.5h14M5 11.5h10M5 16.5h7",
    Models: "M5 7h14v10H5zM8 10h8M8 14h5",
    Runtime: "M6 16l4-8 4 8 4-8",
    Settings: "M12 7v10M7 12h10",
    Doctor: "M12 5v14M5 12h14",
    Logs: "M7 6h10M7 10h10M7 14h7M7 18h4",
  }[name];
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="nav-icon">
      <path d={path} />
    </svg>
  );
}

function RefreshIcon({ spinning }: { spinning: boolean }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className={spinning ? "refresh-icon spinning" : "refresh-icon"}>
      <path d="M20 11a8 8 0 0 0-14.2-4.9M4 5v5h5M4 13a8 8 0 0 0 14.2 4.9M20 19v-5h-5" />
    </svg>
  );
}

function ThemeIcon({ mode }: { mode: ThemeMode }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="button-icon">
      {mode === "light" ? (
        <path d="M12 3v2M12 19v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M3 12h2M19 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4M8 12a4 4 0 1 0 8 0 4 4 0 0 0-8 0" />
      ) : (
        <path d="M20 15.2A7.5 7.5 0 0 1 8.8 4a7.8 7.8 0 1 0 11.2 11.2" />
      )}
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="button-icon">
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  );
}

function PlusIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="button-icon">
      <path d="M12 5v14M5 12h14" />
    </svg>
  );
}

function getInitialTheme(): ThemeMode {
  try {
    const stored = window.localStorage.getItem("vinollama.theme");
    if (stored === "light" || stored === "dark") {
      return stored;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  } catch {
    return "light";
  }
}

function getConversationTitle(messages: ChatMessage[]) {
  const firstUser = messages.find((message) => message.role === "user")?.content.trim();
  const fallback = messages.find((message) => message.content.trim() !== "")?.content.trim() ?? "Desktop conversation";
  return (firstUser || fallback).replace(/\s+/g, " ").slice(0, 64);
}

function isWelcomeMessage(message: ChatMessage) {
  return message.role === "assistant" && message.content === welcomeMessages[0].content;
}

function buildPromptMessages(messages: ChatMessage[], systemPrompt: string) {
  const clean = messages.filter((message) => message.content.trim() !== "" && !isWelcomeMessage(message));
  const prompt = systemPrompt.trim();
  if (!prompt) {
    return clean;
  }
  return [{ role: "system", content: prompt } satisfies ChatMessage, ...clean];
}

function buildStoredMessages(messages: ChatMessage[], systemPrompt: string) {
  return buildPromptMessages(messages, systemPrompt);
}

function splitConversationMessages(messages: ChatMessage[]) {
  const systemPrompt = messages.find((message) => message.role === "system")?.content ?? "";
  const chatMessages = messages.filter((message) => message.role !== "system" && !isWelcomeMessage(message));
  return { systemPrompt, chatMessages };
}

function formatRelativeDate(value?: string) {
  if (!value) {
    return "unknown time";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatBytes(size?: number) {
  if (!size) {
    return "unknown";
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = size;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[index]}`;
}

function flattenLogs(logs: LogsResponse | null): string[] {
  if (!logs) {
    return [];
  }
  if (logs.lines) {
    return logs.lines;
  }
  if (!logs.logs) {
    return [];
  }
  return logs.logs.flatMap((entry) => entry.lines.map((line) => `${entry.file}: ${line}`));
}
