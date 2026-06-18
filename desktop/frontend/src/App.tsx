import { useCallback, useEffect, useMemo, useState, type FormEvent, type KeyboardEvent, type ReactNode } from "react";
import logoUrl from "./assets/vinollama-logo.png";
import {
  ChatMessage,
  ConversationSummary,
  deleteConversation,
  deployDeploymentBinary,
  DeploymentReport,
  DoctorCheck,
  exportConversationMarkdown,
  fetchConversation,
  fetchConversations,
  fetchDeployment,
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
  selectDeploymentBinary,
  sendChatStream,
  stopRuntimeModel,
  updateConversation,
} from "./api";
import { getInitialLanguage, interpolate, languageOptions, translations, type I18nCopy, type Language } from "./i18n";

const navItems = ["Chat", "Models", "Runtime", "Settings", "Doctor", "Logs"] as const;
type NavItem = (typeof navItems)[number];
type ThemeMode = "light" | "dark";

export default function App() {
  const [active, setActive] = useState<NavItem>("Chat");
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme());
  const [language, setLanguage] = useState<Language>(() => getInitialLanguage());
  const copy = translations[language];
  const [service, setService] = useState<ServiceStatus>({
    running: false,
    base_url: "http://127.0.0.1:11435",
  });
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [settings, setSettings] = useState<SettingsStatus | null>(null);
  const [doctor, setDoctor] = useState<DoctorCheck[]>([]);
  const [logs, setLogs] = useState<LogsResponse | null>(null);
  const [deployment, setDeployment] = useState<DeploymentReport | null>(null);
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedModel, setSelectedModel] = useState("");

  const refresh = useCallback(async () => {
    const controller = new AbortController();
    setRefreshing(true);
    try {
      const [nextService, nextRuntime, nextModels, nextSettings, nextDoctor, nextLogs, nextDeployment, nextConversations] = await Promise.all([
        fetchServiceStatus(controller.signal),
        fetchRuntimeStatus(controller.signal),
        fetchModels(controller.signal),
        fetchSettings(controller.signal),
        fetchDoctor(controller.signal),
        fetchLogs(controller.signal),
        fetchDeployment(controller.signal),
        fetchConversations(controller.signal),
      ]);
      setService(nextService);
      setRuntime(nextRuntime);
      setModels(nextModels);
      setSettings(nextSettings);
      setDoctor(nextDoctor);
      setLogs(nextLogs);
      setDeployment(nextDeployment);
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
      return copy.app.offline;
    }
    return runtime?.backend || settings?.runtime?.backend || copy.common.auto;
  }, [copy.app.offline, copy.common.auto, runtime?.backend, service.running, settings?.runtime?.backend]);

  const activeModel = selectedModel || copy.app.noModelSelected;
  const toggleTheme = () => setTheme((current) => (current === "light" ? "dark" : "light"));

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem("vinollama.theme", theme);
  }, [theme]);

  useEffect(() => {
    document.documentElement.lang = language === "zh-CN" ? "zh-CN" : "en";
    window.localStorage.setItem("vinollama.language", language);
  }, [language]);

  return (
    <main className="app-shell" data-theme={theme}>
      <aside className="sidebar" aria-label={copy.app.primaryNav}>
        <div className="brand">
          <img className="brand-mark" src={logoUrl} alt="VinoLlama logo" width="34" height="34" />
          <div>
            <strong>VinoLlama</strong>
            <span>{copy.app.subtitle}</span>
          </div>
        </div>

        <div className="service-card">
          <div className="service-card-row">
            <span className={service.running ? "status-dot online" : "status-dot"} />
            <strong>{service.running ? copy.app.connected : copy.app.offline}</strong>
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
              <span>{copy.nav[item]}</span>
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <span>{copy.app.defaultBind}</span>
          <strong>127.0.0.1:11435</strong>
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div className="crumbs">
            <span>{copy.nav[active]}</span>
            <strong>{active === "Chat" ? activeModel : copy.app.localWorkspace}</strong>
          </div>
          <div className="topbar-actions">
            <span className="runtime-pill">
              {copy.app.backendPrefix} {runtimeLabel}
            </span>
            <span className="runtime-pill">
              {models.length} {copy.app.modelCount}
            </span>
            <label className="language-picker">
              <span>{copy.app.language}</span>
              <select value={language} onChange={(event) => setLanguage(event.target.value as Language)} aria-label={copy.app.language}>
                {languageOptions.map((option) => (
                  <option value={option.code} key={option.code}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className="icon-button"
              onClick={toggleTheme}
              aria-label={interpolate(copy.app.switchTheme, { mode: theme === "light" ? copy.app.dark : copy.app.light })}
              title={interpolate(copy.app.switchTheme, { mode: theme === "light" ? copy.app.dark : copy.app.light })}
            >
              <ThemeIcon mode={theme} />
            </button>
            <button
              type="button"
              className="icon-button"
              onClick={() => void refresh()}
              aria-label={copy.app.refreshStatus}
              title={copy.app.refreshStatus}
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
            copy={copy}
            onSelectModel={setSelectedModel}
            onRefresh={refresh}
          />
        )}
        {active === "Models" && (
          <ModelsPanel
            service={service}
            models={models}
            copy={copy}
            onUseModel={setSelectedModel}
            onNavigate={setActive}
            onRefresh={refresh}
          />
        )}
        {active === "Runtime" && <RuntimePanel service={service} runtime={runtime} settings={settings} copy={copy} onRefresh={refresh} />}
        {active === "Settings" && <SettingsPanel service={service} settings={settings} deployment={deployment} copy={copy} onRefresh={refresh} />}
        {active === "Doctor" && <DoctorPanel service={service} checks={doctor} copy={copy} />}
        {active === "Logs" && <LogsPanel service={service} logs={logs} copy={copy} />}
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
  copy,
  onSelectModel,
  onRefresh,
}: {
  service: ServiceStatus;
  models: ModelInfo[];
  runtime: RuntimeStatus | null;
  settings: SettingsStatus | null;
  conversations: ConversationSummary[];
  selectedModel: string;
  copy: I18nCopy;
  onSelectModel: (model: string) => void;
  onRefresh: () => Promise<void>;
}) {
  const [messages, setMessages] = useState<ChatMessage[]>(() => welcomeMessages(copy));
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
  const visibleModel = selectedModel || copy.chat.selectModel;

  useEffect(() => {
    setMessages((current) => (current.length === 1 && isWelcomeMessage(current[0]) ? welcomeMessages(copy) : current));
  }, [copy]);

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
        setError(copy.chat.generationStopped);
      } else {
        setError(err instanceof Error ? err.message : copy.chat.chatRequestFailed);
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
    setMessages(welcomeMessages(copy));
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
        setSaveNotice(copy.chat.conversationCouldNotLoad);
        return;
      }
      const split = splitConversationMessages(conversation.messages);
      setSystemPrompt(split.systemPrompt);
      setMessages(split.chatMessages.length > 0 ? split.chatMessages : welcomeMessages(copy));
      setActiveConversationId(conversation.id);
      if (conversation.model) {
        onSelectModel(conversation.model);
      }
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : copy.chat.conversationCouldNotLoad);
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
      setSaveNotice(interpolate(copy.chat.savedConversation, { title: conversation.title }));
      await onRefresh();
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : copy.chat.conversationCouldNotSave);
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
        setChatSettingsNotice(copy.chat.checkGeneration);
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
      setChatSettingsNotice(saved.restart_required ? copy.common.savedRestart : copy.common.saved);
      await onRefresh();
    } catch (err) {
      setChatSettingsNotice(err instanceof Error ? err.message : copy.chat.settingsCouldNotSave);
    } finally {
      setSavingChatSettings(false);
    }
  };

  const deleteStoredConversation = async (id: string, title: string) => {
    if (!service.running || isStreaming || !window.confirm(interpolate(copy.chat.deleteConfirm, { title }))) {
      return;
    }
    setBusyConversation(`delete:${id}`);
    setSaveNotice("");
    try {
      await deleteConversation(id);
      if (id === activeConversationId) {
        startNewConversation();
      }
      setSaveNotice(copy.chat.conversationDeleted);
      await onRefresh();
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : copy.chat.conversationCouldNotDelete);
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
      setSaveNotice(copy.chat.exportCopied);
    } catch (err) {
      setSaveNotice(err instanceof Error ? err.message : copy.chat.exportFailed);
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
    <section className={inspectorOpen ? "chat-layout" : "chat-layout inspector-collapsed"} aria-label={copy.chat.workspace}>
      <div className="conversation-rail">
        <button type="button" className="primary-action" disabled={isStreaming} onClick={startNewConversation}>
          <PlusIcon />
          <span>{copy.chat.newChat}</span>
        </button>
        <div className="rail-section">
          <span className="rail-label">{copy.chat.recent}</span>
          {conversations.length === 0 ? (
            <span className="rail-empty">{copy.chat.noSaved}</span>
          ) : (
            conversations.map((conversation) => (
              <div className="conversation-row" key={conversation.id}>
                <button
                  type="button"
                  className={conversation.id === activeConversationId ? "conversation-item active" : "conversation-item"}
                  disabled={!service.running || isStreaming || busyConversation !== ""}
                  onClick={() => void openConversation(conversation.id)}
                >
                  <strong>{conversation.title || copy.chat.untitled}</strong>
                  <span>{conversation.model || copy.chat.unknownModel}</span>
                  <small>{formatRelativeDate(conversation.updated_at)}</small>
                </button>
                <div className="conversation-actions" aria-label={interpolate(copy.chat.actionsFor, { title: conversation.title })}>
                  <button
                    type="button"
                    className="mini-action"
                    disabled={!service.running || busyConversation !== ""}
                    onClick={() => void exportStoredConversation(conversation.id)}
                    title={copy.chat.copyMarkdown}
                    aria-label={`${copy.chat.copyMarkdown} ${conversation.title}`}
                  >
                    MD
                  </button>
                  <button
                    type="button"
                    className="mini-action danger-action"
                    disabled={!service.running || isStreaming || busyConversation !== ""}
                    onClick={() => void deleteStoredConversation(conversation.id, conversation.title || copy.chat.untitled)}
                    title={interpolate(copy.chat.deleteConversation, { title: conversation.title || copy.chat.untitled })}
                    aria-label={interpolate(copy.chat.deleteConversation, { title: conversation.title || copy.chat.untitled })}
                  >
                    {copy.chat.deleteShort}
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
            <span>{copy.chat.model}</span>
            <select
              aria-label={copy.chat.model}
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
            <span className="service-chip">{service.running ? copy.chat.apiOnline : copy.chat.apiOffline}</span>
            <span className="service-chip">
              {runtime?.processes.length ?? 0} {copy.chat.running}
            </span>
            <button
              type="button"
              className="secondary-action compact-action"
              onClick={() => setInspectorOpen((current) => !current)}
              aria-expanded={inspectorOpen}
              aria-controls="chat-settings-panel"
            >
              {inspectorOpen ? copy.chat.hidePanel : copy.chat.showPanel}
            </button>
            <button
              type="button"
              className="secondary-action compact-action"
              disabled={!canSave || busyConversation === "save"}
              onClick={() => void saveCurrentConversation()}
            >
              {busyConversation === "save" ? copy.common.saving : activeConversationId ? copy.chat.save : copy.chat.saveNew}
            </button>
          </div>
        </div>

        <div className="message-surface">
          {messages.map((turn, index) => (
            <article className={`message ${turn.role}`} key={`${turn.role}-${index}`}>
              <span>{turn.role}</span>
              <p>{turn.content || (isStreaming && index === messages.length - 1 ? copy.chat.thinking : "")}</p>
            </article>
          ))}
          {!service.running && (
            <div className="inline-banner">
              <strong>{copy.chat.startApiTitle}</strong>
              <span>{service.error ?? copy.chat.startApiBody}</span>
            </div>
          )}
          {error && (
            <div className="inline-banner warning" role="status">
              <strong>{error}</strong>
              <span>{error === copy.chat.generationStopped ? copy.chat.generationStoppedBody : copy.chat.runtimeErrorBody}</span>
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
            aria-label={copy.chat.message}
            disabled={!service.running || isStreaming}
            placeholder={copy.chat.askPlaceholder}
            rows={3}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={handleComposerKeyDown}
          />
          <div className="composer-actions">
            <button type="button" className="secondary-action" disabled={!isStreaming} onClick={stopMessage}>
              {copy.chat.stop}
            </button>
            <button type="submit" disabled={!canSend}>
              {isStreaming ? copy.chat.sending : copy.chat.send}
            </button>
          </div>
        </form>
      </div>

      <aside className="inspector" id="chat-settings-panel" aria-label={copy.chat.settingsPanel} hidden={!inspectorOpen}>
        <div className="inspector-header">
          <div>
            <span>{copy.chat.localSettings}</span>
            <strong>{copy.chat.chatControls}</strong>
          </div>
          <button
            type="button"
            className="icon-button"
            onClick={() => setInspectorOpen(false)}
            aria-label={copy.chat.hideSettings}
            title={copy.chat.hideSettings}
          >
            <CloseIcon />
          </button>
        </div>
        <form className="inspector-form" onSubmit={saveChatSettings}>
          <label>
            <span>{copy.common.backend}</span>
            <select value={chatBackend} onChange={(event) => setChatBackend(event.target.value)} disabled={!service.running || savingChatSettings}>
              <option value="auto">auto</option>
              <option value="openvino">openvino</option>
              <option value="cpu">cpu</option>
            </select>
          </label>
          <label>
            <span>{copy.chat.context}</span>
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
            <span>{copy.chat.temperature}</span>
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
            <span>{copy.chat.topP}</span>
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
            <span>{copy.chat.threads}</span>
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
            <span>{copy.chat.systemPrompt}</span>
            <textarea
              value={systemPrompt}
              onChange={(event) => setSystemPrompt(event.target.value)}
              placeholder={copy.chat.systemPlaceholder}
              rows={6}
            />
          </label>
          <button type="submit" disabled={!service.running || savingChatSettings}>
            {savingChatSettings ? copy.common.saving : copy.common.saveSettings}
          </button>
          {chatSettingsNotice && <span className="form-note">{chatSettingsNotice}</span>}
        </form>
        <div className="inspector-note">
          <strong>{copy.common.privacy}</strong>
          <span>{copy.chat.privacyBody}</span>
        </div>
      </aside>
    </section>
  );
}

function ModelsPanel({
  service,
  models,
  copy,
  onUseModel,
  onNavigate,
  onRefresh,
}: {
  service: ServiceStatus;
  models: ModelInfo[];
  copy: I18nCopy;
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
      setNotice(interpolate(copy.models.imported, { name: name.trim() }));
      setName("");
      setPath("");
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : copy.models.importFailed);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title={copy.models.title} kicker={service.running ? copy.models.ready : copy.models.waiting} />
      <form className="import-strip" onSubmit={handleImport}>
        <label>
          <span>{copy.models.name}</span>
          <input value={name} onChange={(event) => setName(event.target.value)} disabled={!service.running || busy} />
        </label>
        <label className="path-field">
          <span>{copy.models.ggufPath}</span>
          <input value={path} onChange={(event) => setPath(event.target.value)} disabled={!service.running || busy} />
        </label>
        <label>
          <span>{copy.models.mode}</span>
          <select value={mode} onChange={(event) => setMode(event.target.value as "reference" | "copy" | "link")} disabled={!service.running || busy}>
            <option value="reference">reference</option>
            <option value="copy">copy</option>
            <option value="link">link</option>
          </select>
        </label>
        <button type="submit" disabled={!service.running || busy || !name.trim() || !path.trim()}>
          {busy ? copy.models.importing : copy.models.import}
        </button>
      </form>
      {notice && (
        <div className="inline-banner warning" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <div className="model-grid">
        {models.length === 0 ? (
          <EmptyState title={copy.models.emptyTitle} body={copy.models.emptyBody} />
        ) : (
          models.map((model) => (
            <article className="model-card" key={model.name}>
              <div>
                <strong>{model.name}</strong>
                <span>
                  {model.parameters ?? copy.common.unknown} {copy.models.parameters}
                </span>
              </div>
              <dl>
                <MetricRow label={copy.models.quant} value={model.quantization ?? copy.common.unknown} />
                <MetricRow label={copy.common.backend} value={model.backend_hint ?? copy.common.auto} />
                <MetricRow label={copy.models.size} value={formatBytes(model.size)} />
              </dl>
              <button
                type="button"
                onClick={() => {
                  onUseModel(model.name);
                  onNavigate("Chat");
                }}
              >
                {copy.models.useInChat}
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
  copy,
  onRefresh,
}: {
  service: ServiceStatus;
  runtime: RuntimeStatus | null;
  settings: SettingsStatus | null;
  copy: I18nCopy;
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
      setNotice(error instanceof Error ? error.message : interpolate(copy.runtime.failed, { action }));
    } finally {
      setBusyModel("");
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title={copy.runtime.title} kicker={service.running ? copy.runtime.ready : copy.runtime.offline} />
      <div className="runtime-summary">
        <Metric label={copy.runtime.mode} value={runtime?.backend ?? settings?.runtime?.backend ?? copy.common.auto} />
        <Metric label={copy.runtime.idleTimeout} value={settings?.runtime?.idle_timeout ?? "10m"} />
        <Metric label={copy.runtime.processes} value={`${runtime?.processes.length ?? 0}`} />
      </div>
      <div className="process-table" role="table" aria-label={copy.runtime.tableLabel}>
        <div className="table-row header" role="row">
          <span>{copy.common.model}</span>
          <span>{copy.common.backend}</span>
          <span>{copy.runtime.pid}</span>
          <span>{copy.runtime.port}</span>
          <span>{copy.runtime.state}</span>
          <span>{copy.runtime.actions}</span>
        </div>
        {(runtime?.processes ?? []).length === 0 ? (
          <div className="table-empty">{copy.runtime.noRunning}</div>
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
                  {busyModel === `stop:${process.model}` ? copy.runtime.stopping : copy.runtime.stop}
                </button>
                <button type="button" onClick={() => void runRuntimeAction(process.model, "restart")} disabled={busyModel !== ""}>
                  {busyModel === `restart:${process.model}` ? copy.runtime.restarting : copy.runtime.restart}
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
  deployment,
  copy,
  onRefresh,
}: {
  service: ServiceStatus;
  settings: SettingsStatus | null;
  deployment: DeploymentReport | null;
  copy: I18nCopy;
  onRefresh: () => Promise<void>;
}) {
  const [backend, setBackend] = useState(settings?.runtime?.backend ?? "auto");
  const [idleTimeout, setIdleTimeout] = useState(settings?.runtime?.idle_timeout ?? "10m0s");
  const [readyTimeout, setReadyTimeout] = useState(settings?.runtime?.ready_timeout ?? "30s");
  const [llamaOpenVINOBin, setLlamaOpenVINOBin] = useState(settings?.runtime?.llama_openvino_bin ?? "");
  const [llamaCPUBin, setLlamaCPUBin] = useState(settings?.runtime?.llama_cpu_bin ?? "");
  const [openVINODevice, setOpenVINODevice] = useState(settings?.runtime?.openvino_device ?? "");
  const [healthPath, setHealthPath] = useState(settings?.runtime?.health_path ?? "");
  const [internalPortStart, setInternalPortStart] = useState(`${settings?.runtime?.internal_port_start ?? 21435}`);
  const [extraOpenVINOArgs, setExtraOpenVINOArgs] = useState(formatArgLine(settings?.runtime?.extra_openvino_args));
  const [extraCPUArgs, setExtraCPUArgs] = useState(formatArgLine(settings?.runtime?.extra_cpu_args));
  const [allowUnverifiedFlags, setAllowUnverifiedFlags] = useState(settings?.runtime?.allow_unverified_flags ?? false);
  const [startServiceOnLaunch, setStartServiceOnLaunch] = useState(settings?.desktop?.start_service_on_launch ?? true);
  const [stopServiceOnExit, setStopServiceOnExit] = useState(settings?.desktop?.stop_service_on_exit ?? false);
  const [ctxSize, setCtxSize] = useState(`${settings?.generation?.ctx_size ?? 4096}`);
  const [temperature, setTemperature] = useState(`${settings?.generation?.temperature ?? 0.7}`);
  const [topP, setTopP] = useState(`${settings?.generation?.top_p ?? 0.9}`);
  const [threads, setThreads] = useState(`${settings?.generation?.threads ?? 0}`);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState("");
  const [selectingBinary, setSelectingBinary] = useState("");
  const [deployingBinary, setDeployingBinary] = useState("");

  useEffect(() => {
    setBackend(settings?.runtime?.backend ?? "auto");
    setIdleTimeout(settings?.runtime?.idle_timeout ?? "10m0s");
    setReadyTimeout(settings?.runtime?.ready_timeout ?? "30s");
    setLlamaOpenVINOBin(settings?.runtime?.llama_openvino_bin ?? "");
    setLlamaCPUBin(settings?.runtime?.llama_cpu_bin ?? "");
    setOpenVINODevice(settings?.runtime?.openvino_device ?? "");
    setHealthPath(settings?.runtime?.health_path ?? "");
    setInternalPortStart(`${settings?.runtime?.internal_port_start ?? 21435}`);
    setExtraOpenVINOArgs(formatArgLine(settings?.runtime?.extra_openvino_args));
    setExtraCPUArgs(formatArgLine(settings?.runtime?.extra_cpu_args));
    setAllowUnverifiedFlags(settings?.runtime?.allow_unverified_flags ?? false);
    setStartServiceOnLaunch(settings?.desktop?.start_service_on_launch ?? true);
    setStopServiceOnExit(settings?.desktop?.stop_service_on_exit ?? false);
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
      const parsedCtxSize = Number(ctxSize);
      const parsedTemperature = Number(temperature);
      const parsedTopP = Number(topP);
      const parsedThreads = Number(threads);
      const parsedPortStart = Number(internalPortStart);
      if (
        !Number.isFinite(parsedCtxSize) ||
        parsedCtxSize <= 0 ||
        !Number.isFinite(parsedTemperature) ||
        parsedTemperature < 0 ||
        !Number.isFinite(parsedTopP) ||
        parsedTopP < 0 ||
        parsedTopP > 1 ||
        !Number.isFinite(parsedThreads) ||
        parsedThreads < 0 ||
        !Number.isInteger(parsedPortStart) ||
        parsedPortStart <= 0 ||
        parsedPortStart > 65535
      ) {
        setNotice(copy.settings.checkNumeric);
        return;
      }
      const saved = await saveSettings({
        runtime: {
          backend,
          idle_timeout: idleTimeout,
          ready_timeout: readyTimeout,
          llama_openvino_bin: llamaOpenVINOBin.trim(),
          llama_cpu_bin: llamaCPUBin.trim(),
          openvino_device: openVINODevice.trim(),
          internal_port_start: parsedPortStart,
          health_path: healthPath.trim(),
          extra_openvino_args: parseArgLine(extraOpenVINOArgs),
          extra_cpu_args: parseArgLine(extraCPUArgs),
          allow_unverified_flags: allowUnverifiedFlags,
        },
        generation: {
          ctx_size: parsedCtxSize,
          temperature: parsedTemperature,
          top_p: parsedTopP,
          threads: parsedThreads,
        },
        privacy: {
          telemetry: false,
        },
        desktop: {
          start_service_on_launch: startServiceOnLaunch,
          stop_service_on_exit: stopServiceOnExit,
        },
      });
      setNotice(saved.restart_required ? copy.common.savedRestart : copy.common.saved);
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : copy.settings.couldNotSave);
    } finally {
      setSaving(false);
    }
  };

  const useDeploymentBinary = async (kind: string, path: string) => {
    setSelectingBinary(`${kind}:${path}`);
    setNotice("");
    try {
      await selectDeploymentBinary(kind, path);
      setNotice(interpolate(copy.settings.selectedBinary, { kind }));
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : copy.settings.couldNotSave);
    } finally {
      setSelectingBinary("");
    }
  };

  const deployRecommendedBinary = async (kind: string, path: string) => {
    const key = `${kind}:${path}`;
    setDeployingBinary(key);
    setNotice("");
    try {
      await deployDeploymentBinary(kind, path);
      setNotice(`${kind} backend deployed to the managed VinoLlama runtime directory.`);
      await onRefresh();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : copy.settings.couldNotSave);
    } finally {
      setDeployingBinary("");
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title={copy.settings.title} kicker={service.running ? copy.settings.active : copy.settings.preview} />
      <form className="settings-grid" onSubmit={submitSettings}>
        <SettingGroup title={copy.settings.server}>
          <MetricRow label={copy.settings.host} value={settings?.server?.host ?? "127.0.0.1"} />
          <MetricRow label={copy.settings.port} value={`${settings?.server?.port ?? 11435}`} />
        </SettingGroup>
        <SettingGroup title={copy.settings.generation}>
          <EditableRow label={copy.settings.context} value={ctxSize} onChange={setCtxSize} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.temperature} value={temperature} onChange={setTemperature} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.topP} value={topP} onChange={setTopP} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.threads} value={threads} onChange={setThreads} disabled={!service.running || saving} />
        </SettingGroup>
        <SettingGroup title={copy.settings.runtime}>
          <label className="editable-row">
            <span>{copy.common.backend}</span>
            <select value={backend} onChange={(event) => setBackend(event.target.value)} disabled={!service.running || saving}>
              <option value="auto">{copy.common.auto}</option>
              <option value="openvino">openvino</option>
              <option value="cpu">cpu</option>
            </select>
          </label>
          <EditableRow label={copy.settings.idleTimeout} value={idleTimeout} onChange={setIdleTimeout} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.readyTimeout} value={readyTimeout} onChange={setReadyTimeout} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.internalPortStart} value={internalPortStart} onChange={setInternalPortStart} disabled={!service.running || saving} />
        </SettingGroup>
        <SettingGroup title={copy.settings.desktopService}>
          <label className="checkbox-row">
            <input
              type="checkbox"
              checked={startServiceOnLaunch}
              onChange={(event) => setStartServiceOnLaunch(event.target.checked)}
              disabled={!service.running || saving}
            />
            <span>{copy.settings.startServiceOnLaunch}</span>
          </label>
          <label className="checkbox-row">
            <input
              type="checkbox"
              checked={stopServiceOnExit}
              onChange={(event) => setStopServiceOnExit(event.target.checked)}
              disabled={!service.running || saving}
            />
            <span>{copy.settings.stopServiceOnExit}</span>
          </label>
        </SettingGroup>
        <SettingGroup title={copy.settings.backendBinaries} wide>
          <EditableRow
            label={copy.settings.openVINOBinary}
            value={llamaOpenVINOBin}
            onChange={setLlamaOpenVINOBin}
            disabled={!service.running || saving}
          />
          <EditableRow label={copy.settings.cpuBinary} value={llamaCPUBin} onChange={setLlamaCPUBin} disabled={!service.running || saving} />
          <label className="editable-row">
            <span>{copy.settings.openVINODevice}</span>
            <input
              list="openvino-device-options"
              value={openVINODevice}
              onChange={(event) => setOpenVINODevice(event.target.value)}
              disabled={!service.running || saving}
              placeholder={copy.settings.openVINODevicePlaceholder}
            />
            <datalist id="openvino-device-options">
              <option value="CPU" />
              <option value="GPU" />
              <option value="NPU" />
              <option value="GPU.0" />
              <option value="GPU.1" />
            </datalist>
          </label>
        </SettingGroup>
        <SettingGroup title={copy.settings.deployment} wide>
          {deployment ? (
            <div className="deployment-panel">
              <div className="deployment-row">
                <strong>{copy.settings.deploymentReady}</strong>
                <span className={deployment.readiness === "ready" ? "status-text ok" : "status-text warn"}>
                  {deployment.readiness || copy.common.unknown}
                </span>
                {deployment.managed?.root && <small>Managed runtime directory: {deployment.managed.root}</small>}
                {deployment.actions && deployment.actions.length > 0 && (
                  <div className="deployment-action-list">
                    {deployment.actions.map((action) => {
                      const canDeploy = action.kind === "deploy_binary" && action.path && action.backend && action.status !== "ready";
                      const key = `${action.backend}:${action.path || action.id}`;
                      return (
                        <article className="deployment-action-card" key={action.id}>
                          <div>
                            <span className={action.status === "ready" ? "status-chip ok" : "status-chip warn"}>{action.status}</span>
                            <strong>{action.title}</strong>
                            <small>{action.summary}</small>
                            {action.install_dir && <code>{action.install_dir}</code>}
                            {action.docs_url && (
                              <a href={action.docs_url} target="_blank" rel="noreferrer">
                                Open official guide
                              </a>
                            )}
                          </div>
                          {canDeploy && (
                            <button
                              type="button"
                              className="secondary-action compact-action"
                              disabled={!service.running || deployingBinary !== ""}
                              onClick={() => void deployRecommendedBinary(action.backend || "", action.path || "")}
                            >
                              {deployingBinary === key ? "Deploying..." : action.button_label || "Deploy"}
                            </button>
                          )}
                        </article>
                      );
                    })}
                  </div>
                )}
              </div>
              <div className="deployment-row">
                <strong>{copy.settings.openVINORuntime}</strong>
                <span className={deployment.openvino.found ? "status-text ok" : "status-text warn"}>
                  {deployment.openvino.found ? copy.settings.found : copy.settings.missing}
                </span>
                <small>{deployment.openvino.path || deployment.openvino.setup_script || deployment.openvino.fix}</small>
              </div>
              <div className="deployment-row">
                <strong>{copy.settings.buildTools}</strong>
                <div className="tool-strip">
                  {deployment.tools.map((tool) => (
                    <span className={tool.found ? "status-chip ok" : "status-chip warn"} key={tool.name} title={tool.path || tool.fix}>
                      {tool.name}
                    </span>
                  ))}
                </div>
              </div>
              <div className="deployment-row">
                <strong>{copy.settings.binaryCandidates}</strong>
                {deployment.binaries.length === 0 ? (
                  <small>{copy.settings.noCandidates}</small>
                ) : (
                  <div className="binary-list">
                    {deployment.binaries.map((binary) => {
                      const key = `${binary.kind}:${binary.path}`;
                      const canSelect = binary.usable && (binary.kind !== "openvino" || binary.openvino_capable);
                      return (
                        <div className="binary-row" key={key}>
                          <div>
                            <span className={canSelect ? "status-chip ok" : "status-chip warn"}>{binary.kind}</span>
                            <code>{binary.path}</code>
                            <small>{binary.reason || binary.version || binary.source}</small>
                          </div>
                          <button
                            type="button"
                            className="secondary-action compact-action"
                            disabled={!service.running || !canSelect || selectingBinary !== ""}
                            onClick={() => void useDeploymentBinary(binary.kind, binary.path)}
                          >
                            {selectingBinary === key
                              ? copy.settings.selecting
                              : binary.kind === "openvino"
                                ? copy.settings.useForOpenVINO
                                : copy.settings.useForCPU}
                          </button>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
              <div className="deployment-row">
                <strong>{copy.settings.recommendations}</strong>
                {deployment.recommendations.map((item) => (
                  <small key={item}>{item}</small>
                ))}
              </div>
              <details className="build-plan-details">
                <summary>{copy.settings.buildPlans}</summary>
                {deployment.build_plans.map((plan) => (
                  <div className="build-plan" key={plan.name}>
                    <strong>{plan.name}</strong>
                    <small>{plan.description}</small>
                    {plan.steps.map((step) => (
                      <code key={`${plan.name}:${step.command}`}>{step.command}</code>
                    ))}
                  </div>
                ))}
              </details>
              <a href={deployment.reference} target="_blank" rel="noreferrer">
                {copy.settings.deploymentReference}
              </a>
            </div>
          ) : (
            <div className="deployment-panel">
              <small>{copy.settings.noDeployment}</small>
            </div>
          )}
        </SettingGroup>
        <SettingGroup title={copy.settings.advancedRuntime} wide>
          <EditableRow label={copy.settings.healthPath} value={healthPath} onChange={setHealthPath} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.openVINOExtraArgs} value={extraOpenVINOArgs} onChange={setExtraOpenVINOArgs} disabled={!service.running || saving} />
          <EditableRow label={copy.settings.cpuExtraArgs} value={extraCPUArgs} onChange={setExtraCPUArgs} disabled={!service.running || saving} />
          <label className="checkbox-row">
            <input
              type="checkbox"
              checked={allowUnverifiedFlags}
              onChange={(event) => setAllowUnverifiedFlags(event.target.checked)}
              disabled={!service.running || saving}
            />
            <span>{copy.settings.allowUnverified}</span>
          </label>
        </SettingGroup>
        <SettingGroup title={copy.common.privacy}>
          <MetricRow label={copy.settings.telemetry} value={settings?.privacy?.telemetry ? copy.common.enabled : copy.common.disabled} />
          <MetricRow label={copy.settings.cloudSync} value={copy.common.notImplemented} />
        </SettingGroup>
        <div className="settings-actions">
          <button type="submit" disabled={!service.running || saving}>
            {saving ? copy.common.saving : copy.common.saveSettings}
          </button>
          {notice && <span>{notice}</span>}
        </div>
      </form>
    </section>
  );
}

function DoctorPanel({ service, checks, copy }: { service: ServiceStatus; checks: DoctorCheck[]; copy: I18nCopy }) {
  const [notice, setNotice] = useState("");
  const copyReport = async () => {
    const report = checks
      .map((check) => `[${check.level}] ${check.name}\n${copy.doctor.reason}: ${check.reason || check.what || ""}\n${copy.doctor.fix}: ${check.fix || ""}`)
      .join("\n\n");
    try {
      await navigator.clipboard.writeText(report || copy.doctor.noReport);
      setNotice(copy.doctor.copied);
    } catch {
      setNotice(copy.doctor.clipboardUnavailable);
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title={copy.doctor.title} kicker={service.running ? copy.doctor.ready : copy.doctor.offline}>
        <button type="button" className="secondary-action compact-action" onClick={() => void copyReport()}>
          {copy.doctor.copyReport}
        </button>
      </PageHeader>
      {notice && (
        <div className="inline-banner" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <div className="doctor-list">
        {checks.length === 0 ? (
          <EmptyState title={copy.doctor.noReportTitle} body={copy.doctor.noReportBody} />
        ) : (
          checks.map((check) => (
            <article className="doctor-row" key={`${check.name || "check"}-${check.level || "unknown"}`}>
              <span className={`level ${(check.level || "unknown").toLowerCase()}`}>{check.level || "unknown"}</span>
              <div>
                <strong>{check.name}</strong>
                <p>{check.reason || check.what || copy.doctor.completed}</p>
                {check.fix && <small>{check.fix}</small>}
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}

function LogsPanel({ service, logs, copy }: { service: ServiceStatus; logs: LogsResponse | null; copy: I18nCopy }) {
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState("");
  const lines = flattenLogs(logs);
  const filtered = query.trim()
    ? lines.filter((line) => line.toLowerCase().includes(query.trim().toLowerCase()))
    : lines;
  const copyLogs = async () => {
    try {
      await navigator.clipboard.writeText(filtered.join("\n"));
      setNotice(copy.logs.copied);
    } catch {
      setNotice(copy.logs.clipboardUnavailable);
    }
  };

  return (
    <section className="page-panel">
      <PageHeader title={copy.logs.title} kicker={service.running ? logs?.log_dir ?? copy.logs.tail : copy.logs.offline}>
        <div className="toolbar-cluster">
          <input aria-label={copy.logs.filter} placeholder={copy.logs.filter} value={query} onChange={(event) => setQuery(event.target.value)} />
          <button type="button" className="secondary-action compact-action" onClick={() => void copyLogs()}>
            {copy.common.copy}
          </button>
        </div>
      </PageHeader>
      {notice && (
        <div className="inline-banner" role="status">
          <strong>{notice}</strong>
        </div>
      )}
      <pre className="log-view" aria-label={copy.logs.viewLabel}>
        {filtered.length === 0 ? copy.logs.empty : filtered.join("\n")}
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

function SettingGroup({ title, children, wide = false }: { title: string; children: ReactNode; wide?: boolean }) {
  return (
    <section className={wide ? "setting-group wide" : "setting-group"}>
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
  return message.role === "assistant" && Object.values(translations).some((entry) => message.content === entry.welcome);
}

function welcomeMessages(copy: I18nCopy): ChatMessage[] {
  return [{ role: "assistant", content: copy.welcome }];
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

function formatArgLine(args?: string[]) {
  return args?.join(" ") ?? "";
}

function parseArgLine(value: string) {
  return value
    .split(/\s+/)
    .map((item) => item.trim())
    .filter(Boolean);
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
