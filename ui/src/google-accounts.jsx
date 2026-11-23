import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  Globe2,
  Monitor,
  Pencil,
  Plus,
  RefreshCw,
  TerminalSquare,
  Trash2,
} from "lucide-react";
import { api } from "./api";
import { InlineSelect } from "./inline-select";
import { DesktopToolbar, useDesktopZoom } from "./claude-desktop";
import { SafetyMessage } from "./safety-message";
import { RemoteView } from "./views";
import { useWindowVisibility } from "./window-visibility";
import { useTranslation } from "./interface-language.mjs";
import "./google-accounts.css";

export function useGoogleAccounts(running, ready, jobID) {
  const [state, setState] = useState({
    supported: true,
    vm_running: false,
    antigravity_installed: false,
    accounts: [],
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const reqRef = useRef(null);

  const refresh = useCallback(async () => {
    const req = (reqRef.current ??= api("/google-accounts"));
    setLoading(true);
    try {
      const next = await req;
      setState({
        supported: next?.supported !== false,
        vm_running: Boolean(next?.vm_running),
        antigravity_installed: Boolean(next?.antigravity_installed),
        accounts: Array.isArray(next?.accounts) ? next.accounts : [],
      });
      setError("");
      return next;
    } catch (e) {
      setError(e.message);
      return null;
    } finally {
      if (reqRef.current === req) reqRef.current = null;
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [running, ready, jobID, refresh]);

  const mutate = useCallback(async (payload) => {
    const res = await api("/google-accounts", payload);
    await refresh();
    return res;
  }, [refresh]);

  return {
    supported: state.supported,
    vmRunning: state.vm_running,
    antigravityInstalled: state.antigravity_installed,
    accounts: state.accounts,
    loading,
    error,
    refresh,
    mutate,
  };
}

function accountCaption(account) {
  const name = account.label || account.email || account.detected_email || "Google";
  return account.number ? `#${String(account.number).padStart(2, "0")} ${name}` : name;
}

function GoogleAccountCard({
  account,
  proxySources,
  desktopNetwork,
  running,
  busy,
  antigravityInstalled,
  perform,
  onMutate,
  onOpenAntigravity,
  onOpenBrowser,
  onOpenTerminal,
  onStopSession,
  onConfirm,
}) {
  const tr = useTranslation();
  const [renaming, setRenaming] = useState(false);
  const [label, setLabel] = useState("");
  const displayName = account.label || account.email || account.detected_email || "Google";
  const email = account.email || account.detected_email || "";
  const anyRunning = account.antigravity_running || account.browser_running;

  return (
    <article className="google-profile" aria-label={accountCaption(account)}>
      <div className="google-profile-heading">
        <span className="google-profile-avatar" aria-hidden="true">G</span>
        <div className="google-profile-identity">
          {renaming ? (
            <form
              className="google-profile-rename"
              onSubmit={(e) => {
                e.preventDefault();
                const next = label.trim();
                if (!next) return;
                perform(async () => {
                  await onMutate({ action: "update", id: account.id, label: next });
                  setRenaming(false);
                });
              }}
            >
              <input
                autoFocus
                aria-label={tr("账号备注")}
                maxLength={64}
                placeholder={email || tr("账号名称")}
                value={label}
                onChange={(e) => setLabel(e.target.value)}
              />
              <button disabled={busy || !label.trim()} aria-label={tr("保存备注")}>
                <Check size={15} />
              </button>
              <button type="button" disabled={busy} onClick={() => setRenaming(false)}>
                {tr("取消")}
              </button>
            </form>
          ) : (
            <div className="google-profile-name">
              <h4 title={accountCaption(account)}>{accountCaption(account)}</h4>
              <button
                type="button"
                className="icon-button"
                disabled={busy}
                aria-label={tr("修改备注")}
                onClick={() => {
                  setLabel(account.label || "");
                  setRenaming(true);
                }}
              >
                <Pencil size={14} />
              </button>
            </div>
          )}
          <div className="google-profile-meta">
            {email && <span className="google-profile-email" title={email}>{email}</span>}
            {account.antigravity_running && (
              <span className="google-badge running">● Antigravity {tr("运行中")}</span>
            )}
            {account.browser_running && (
              <span className="google-badge running">● {tr("独立浏览器运行中")}</span>
            )}
            {!anyRunning && account.has_profile && (
              <span className="google-badge ready">{tr("已初始化隔离配置")}</span>
            )}
            {!anyRunning && !account.has_profile && (
              <span className="google-badge">{tr("首次使用时登录")}</span>
            )}
          </div>
        </div>
        <button
          type="button"
          className="icon-button"
          disabled={busy || anyRunning}
          title={anyRunning ? tr("请先关闭该账号运行中的窗口") : tr("移除账号")}
          aria-label={tr("移除账号")}
          onClick={async () => {
            if (
              await onConfirm({
                title: `移除 Google 账号「${displayName}」？`,
                body: "此账号会从列表中移除，虚拟机中的隔离配置目录会保留。",
                confirm: "移除账号",
                danger: true,
              })
            ) {
              perform(() => onMutate({ action: "delete", id: account.id }));
            }
          }}
        >
          <Trash2 size={15} />
        </button>
      </div>

      {desktopNetwork && (
        <label className="google-proxy-row">
          <span>{tr("账号出口")}</span>
          <InlineSelect
            inline
            aria-label={`${displayName} ${tr("的代理节点")}`}
            value={account.proxy_node_id || ""}
            disabled={busy || anyRunning}
            onValueChange={(node) =>
              perform(() => onMutate({ action: "update", id: account.id, proxy_node_id: node }))
            }
          >
            <option value="">{tr("跟随环境出口")}</option>
            {account.proxy_node_id &&
              !proxySources.some((src) => src.nodes.some((n) => n.id === account.proxy_node_id)) && (
                <option value={account.proxy_node_id} disabled>
                  {tr("已绑定节点不可用")}
                </option>
              )}
            {proxySources.map((source) => (
              <optgroup label={source.name} key={source.id}>
                {source.nodes.map((node) => (
                  <option value={node.id} key={node.id}>
                    {node.name}
                  </option>
                ))}
              </optgroup>
            ))}
          </InlineSelect>
        </label>
      )}

      <div className="google-profile-actions">
        <button
          className="primary"
          disabled={busy || !running || !antigravityInstalled}
          title={!antigravityInstalled ? tr("请先在 AI Desktops 中安装 Google Antigravity") : undefined}
          onClick={() => onOpenAntigravity(account)}
        >
          <Monitor size={14} />
          {account.antigravity_running ? tr("切换到 Antigravity") : tr("打开 Antigravity")}
        </button>
        <button
          className="quiet-button"
          disabled={busy || !running}
          onClick={() => onOpenBrowser(account)}
        >
          <Globe2 size={14} />
          {account.browser_running ? tr("切换到独立浏览器") : tr("独立浏览器")}
        </button>
        <button
          className="quiet-button"
          disabled={busy || !running}
          onClick={() => onOpenTerminal(account)}
        >
          <TerminalSquare size={14} />
          {tr("agy / gemini 终端")}
        </button>
        {account.antigravity_running && (
          <button
            className="text-button"
            disabled={busy}
            onClick={() => onStopSession(account, "antigravity")}
          >
            {tr("关闭 Antigravity")}
          </button>
        )}
        {account.browser_running && (
          <button
            className="text-button"
            disabled={busy}
            onClick={() => onStopSession(account, "browser")}
          >
            {tr("关闭浏览器")}
          </button>
        )}
      </div>
    </article>
  );
}

export function GoogleAccountsPanel({
  store,
  proxySources,
  desktopNetwork,
  running,
  environmentBusy,
  onOpenAntigravity,
  onOpenBrowser,
  onOpenTerminal,
  onStopSession,
  onConfirm,
}) {
  const tr = useTranslation();
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);
  const [label, setLabel] = useState("");
  const [email, setEmail] = useState("");
  const [node, setNode] = useState("");
  const [actionError, setActionError] = useState("");

  useEffect(() => {
    if (running) void store.refresh();
  }, [running, store.refresh]);

  const perform = async (operation) => {
    if (busy) return;
    setBusy(true);
    setActionError("");
    try {
      await operation();
    } catch (e) {
      setActionError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const handleCreate = (e) => {
    e.preventDefault();
    perform(async () => {
      await store.mutate({
        action: "create",
        label: label.trim(),
        email: email.trim(),
        proxy_node_id: node,
      });
      setLabel("");
      setEmail("");
      setNode("");
      setAdding(false);
    });
  };

  return (
    <section className="google-profiles" aria-label={tr("Google 账号管理")}>
      <div className="google-profiles-header">
        <p>
          {tr("为每个 Google 账号分配独立的隔离目录、系统密钥环与可选代理出口，同时支持多开 Google Antigravity、独立浏览器（Gemini / AI Studio / NotebookLM）以及 agy / gemini CLI 终端。")}
        </p>
        <div className="google-profiles-toolbar">
          <button
            className="quiet-button"
            disabled={store.loading}
            onClick={() => void store.refresh()}
          >
            <RefreshCw size={14} className={store.loading ? "spin" : undefined} />
            {tr("刷新状态")}
          </button>
          <button
            className="primary"
            disabled={busy || environmentBusy}
            aria-expanded={adding}
            onClick={() => setAdding((v) => !v)}
          >
            <Plus size={15} />
            {tr("添加 Google 账号")}
          </button>
        </div>
      </div>

      {adding && (
        <form className="google-add-form" onSubmit={handleCreate}>
          <label>
            <span>{tr("账号备注")}</span>
            <input
              autoFocus
              maxLength={64}
              placeholder={tr("例如：主力号 / AI Pro 美区（留空自动编号）")}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          </label>
          <label>
            <span>{tr("Google 邮箱（可选，登录后也会自动识别）")}</span>
            <input
              maxLength={120}
              placeholder="name@gmail.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          {desktopNetwork && proxySources.length > 0 && (
            <label>
              <span>{tr("账号出口")}</span>
              <InlineSelect inline aria-label={tr("新 Google 账号出口")} value={node} onValueChange={setNode}>
                <option value="">{tr("跟随环境出口")}</option>
                {proxySources.map((source) => (
                  <optgroup key={source.id} label={source.name}>
                    {source.nodes.map((n) => (
                      <option value={n.id} key={n.id}>
                        {n.name}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </InlineSelect>
            </label>
          )}
          <div className="google-add-form-actions">
            <button className="primary" type="submit" disabled={busy}>
              {tr("创建账号")}
            </button>
            <button type="button" onClick={() => setAdding(false)}>
              {tr("取消")}
            </button>
          </div>
        </form>
      )}

      {!running && (
        <div className="connection-banner" role="status">
          {tr("启动 Cage 环境后，可使用各个 Google 账号独立打开 Antigravity、浏览器或 agy / gemini 终端。")}
        </div>
      )}

      {running && !store.antigravityInstalled && (
        <div className="connection-banner" role="status">
          {tr("尚未安装 Google Antigravity。请先在左侧「AI Desktops」中下载安装，独立浏览器与终端可直接使用。")}
        </div>
      )}

      {(actionError || store.error) && (
        <p className="account-error" role="alert">
          {actionError || store.error}
        </p>
      )}

      <div className="google-profile-list">
        {store.accounts.length === 0 ? (
          <div className="google-profile-empty">
            <p>{tr("尚未添加 Google 账号")}</p>
            <small>{tr("点击上方「添加 Google 账号」创建独立隔离配置，即可在左侧 Google Antigravity 下多开多个账号窗口。")}</small>
          </div>
        ) : (
          store.accounts.map((account) => (
            <GoogleAccountCard
              key={account.id}
              account={account}
              proxySources={proxySources}
              desktopNetwork={desktopNetwork}
              running={running}
              busy={busy || environmentBusy}
              antigravityInstalled={store.antigravityInstalled}
              perform={perform}
              onMutate={store.mutate}
              onOpenAntigravity={onOpenAntigravity}
              onOpenBrowser={onOpenBrowser}
              onOpenTerminal={onOpenTerminal}
              onStopSession={onStopSession}
              onConfirm={onConfirm}
            />
          ))
        )}
      </div>
    </section>
  );
}

export function GoogleSessionView({
  account,
  mode,
  status,
  visible,
  remoteRef,
  onStatus,
  onFallback,
  onConfirm,
  onDetails,
  onExpand,
  sidebarCollapsed,
  onRefreshAccounts,
}) {
  const tr = useTranslation();
  const [zoom, setZoom] = useDesktopZoom();
  const windowVisible = useWindowVisibility();
  const screenVisible = visible && windowVisible;
  const [sessionState, setSessionState] = useState(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [jobID, setJobID] = useState("");
  const opened = useRef(false);

  const accountId = account?.id || "";
  const modeLabel = mode === "browser" ? tr("独立浏览器") : "Google Antigravity";
  const title = `${modeLabel} · ${account?.label || account?.email || accountId}`;
  const vncPrefix = `/google-accounts/${accountId}/${mode}`;
  const ownJob = status.job?.kind?.startsWith(`google-${mode}-${accountId}-`);
  const busy = submitting || Boolean(status.job?.running) || Boolean(jobID);

  useEffect(() => {
    opened.current = false;
    setSessionState(null);
    setError("");
    setJobID("");
  }, [accountId, mode]);

  const refresh = useCallback(async () => {
    if (!accountId) return null;
    try {
      const value = await api(`/google-accounts/${accountId}/${mode}/status`);
      setSessionState(value);
      setError("");
      return value;
    } catch (e) {
      setError(e.message);
      return null;
    }
  }, [accountId, mode]);

  const run = async (action, body = {}) => {
    setSubmitting(true);
    setError("");
    try {
      const job = await api(`/google-accounts/${accountId}/${mode}/${action}`, body);
      setJobID(job.id);
    } catch (e) {
      setError(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  useEffect(() => {
    if (!screenVisible || !status.vm_running || status.job?.running || !accountId) return;
    let disposed = false;
    let timer;
    const poll = async () => {
      const value = await refresh();
      if (disposed) return;
      if (value && !opened.current) {
        opened.current = true;
        if (value.installed && !value.running && !jobID) {
          await run("start");
        }
      }
      if (!disposed) timer = setTimeout(poll, 5000);
    };
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [accountId, mode, screenVisible, status.vm_running, status.job?.running, refresh]);

  useEffect(() => {
    if (!jobID || status.job?.id !== jobID || status.job.running) return;
    setJobID("");
    onRefreshAccounts?.();
    if (status.job.error) {
      setError(status.job.error);
      return;
    }
    void refresh();
  }, [status.job, jobID, onRefreshAccounts, refresh]);

  return (
    <section className="claude-desktop-view" aria-label={title}>
      <DesktopToolbar
        desktop={sessionState}
        platform="Linux"
        title={title}
        zoom={zoom}
        setZoom={setZoom}
        onConfirm={onConfirm}
        onStop={() => run("stop")}
        busy={busy}
        refresh={() => refresh()}
        sidebarCollapsed={sidebarCollapsed}
        onExpand={onExpand}
      />

      {error && (
        <div className="connection-banner" role="alert">
          <SafetyMessage message={error} />
          <button onClick={onDetails}>{tr("操作详情")}</button>
        </div>
      )}

      {busy && ownJob && (
        <div className="desktop-progress" role="status">
          <RefreshCw size={16} className="spin" />
          {status.job.message}
          <button className="text-button" onClick={onDetails}>
            {tr("查看进度")}
          </button>
        </div>
      )}

      <div className="desktop-screen">
        {sessionState?.running && status.vm_running ? (
          <RemoteView
            key={`${accountId}:${mode}`}
            ref={remoteRef}
            desktop
            vncPrefix={vncPrefix}
            zoom={zoom}
            visible={visible}
            keyboard={status.keyboard}
            onStatus={onStatus}
            onFallback={onFallback}
          />
        ) : (
          <div className="desktop-empty">
            {mode === "browser" ? <Globe2 size={36} /> : <Monitor size={36} />}
            <h2>{title}</h2>
            <p>
              {mode === "browser"
                ? tr("此浏览器运行在该 Google 账号的独立隔离环境与专属代理出口中，适用于 Gemini Web、AI Studio 与 NotebookLM。")
                : tr("此 Antigravity 实例运行在该 Google 账号的独立隔离目录与专属代理出口中，可与其他账号并行多开。")}
            </p>
            <div className="desktop-actions">
              <button
                className="primary"
                disabled={busy || !status.vm_running || (sessionState && !sessionState.installed)}
                onClick={() => run("start")}
              >
                {mode === "browser" ? <Globe2 size={16} /> : <Monitor size={16} />}
                {tr("启动会话")}
              </button>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
